package minogrpc

import (
	context "context"
	"sync"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/metadata"
)

// RPC represents an RPC that has been registered by a client, which allows
// clients to call an RPC that will execute the provided handler.
//
// - implements mino.RPC
type RPC struct {
	overlay overlay
	uri     string
	factory serde.Factory
}

// Call implements mino.RPC. It calls the RPC on each provided address.
func (rpc *RPC) Call(ctx context.Context,
	req serde.Message, players mino.Players) (<-chan mino.Response, error) {

	data, err := req.Serialize(rpc.overlay.context)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal msg to any: %v", err)
	}

	from, err := rpc.overlay.me.MarshalText()
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal address: %v", err)
	}

	sendMsg := &Message{
		From:    from,
		Payload: data,
	}

	out := make(chan mino.Response, players.Len())

	wg := sync.WaitGroup{}
	wg.Add(players.Len())

	iter := players.AddressIterator()
	for iter.HasNext() {
		addr := iter.GetNext()

		go func() {
			defer wg.Done()

			clientConn, err := rpc.overlay.connFactory.FromAddress(addr)
			if err != nil {
				resp := mino.NewResponseWithError(
					addr,
					xerrors.Errorf("failed to get client conn: %v", err),
				)

				out <- resp
				return
			}

			cl := NewOverlayClient(clientConn)

			header := metadata.New(map[string]string{headerURIKey: rpc.uri})
			newCtx := metadata.NewOutgoingContext(ctx, header)

			callResp, err := cl.Call(newCtx, sendMsg)
			if err != nil {
				resp := mino.NewResponseWithError(
					addr,
					xerrors.Errorf("failed to call client: %v", err),
				)

				out <- resp
				return
			}

			resp, err := rpc.factory.Deserialize(rpc.overlay.context, callResp.GetPayload())
			if err != nil {
				resp := mino.NewResponseWithError(
					addr,
					xerrors.Errorf("couldn't unmarshal payload: %v", err),
				)

				out <- resp
				return
			}

			out <- mino.NewResponse(addr, resp)
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out, nil
}

// Stream implements mino.RPC. TODO: errors
func (rpc RPC) Stream(ctx context.Context,
	players mino.Players) (mino.Sender, mino.Receiver, error) {

	root := newRootAddress()

	rting, err := rpc.overlay.routingFactory.Make(rpc.overlay.me, players)
	if err != nil {
		return nil, nil, xerrors.Errorf("couldn't generate routing: %v", err)
	}

	header := metadata.New(map[string]string{headerURIKey: rpc.uri})

	receiver := receiver{
		context:        rpc.overlay.context,
		factory:        rpc.factory,
		addressFactory: rpc.overlay.routingFactory.GetAddressFactory(),
		errs:           make(chan error, 1),
		queue:          newNonBlockingQueue(),
	}

	gateway := rting.GetRoot()

	sender := sender{
		me:             root,
		context:        rpc.overlay.context,
		addressFactory: AddressFactory{},
		gateway:        gateway,
		clients:        map[mino.Address]chan OutContext{},
		receiver:       &receiver,
		traffic:        rpc.overlay.traffic,
	}

	relayCtx := metadata.NewOutgoingContext(ctx, header)

	// The orchestrator opens a connection to the entry point of the routing map
	// and it will relay the messages by this gateway by default. The entry
	// point of the routing will have the orchestrator stream opens which will
	// allow the messages to be routed back to the orchestrator.
	err = rpc.overlay.setupRelay(relayCtx, gateway, &sender, &receiver, rting)
	if err != nil {
		return nil, nil, xerrors.Errorf("couldn't setup relay: %v", err)
	}

	return sender, receiver, nil
}
