package minogrpc

import (
	context "context"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/mino"
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
}

// Call implements mino.RPC. It calls the RPC on each provided address.
func (rpc *RPC) Call(ctx context.Context, req proto.Message,
	players mino.Players) (<-chan proto.Message, <-chan error) {

	out := make(chan proto.Message, players.Len())
	errs := make(chan error, players.Len())

	m, err := rpc.overlay.encoder.MarshalAny(req)
	if err != nil {
		errs <- xerrors.Errorf("failed to marshal msg to any: %v", err)
		return out, errs
	}

	sendMsg := &Message{Payload: m}

	go func() {
		iter := players.AddressIterator()
		for iter.HasNext() {
			addr := iter.GetNext()

			clientConn, err := rpc.overlay.connFactory.FromAddress(addr)
			if err != nil {
				errs <- xerrors.Errorf("failed to get client conn for '%v': %v",
					addr, err)
				continue
			}

			cl := NewOverlayClient(clientConn)

			header := metadata.New(map[string]string{headerURIKey: rpc.uri})
			newCtx := metadata.NewOutgoingContext(ctx, header)

			callResp, err := cl.Call(newCtx, sendMsg)
			if err != nil {
				errs <- xerrors.Errorf("failed to call client '%s': %v", addr, err)
				continue
			}

			resp, err := rpc.overlay.encoder.UnmarshalDynamicAny(callResp.GetPayload())
			if err != nil {
				errs <- xerrors.Errorf("couldn't unmarshal payload: %v", err)
				continue
			}

			out <- resp
		}

		close(out)
	}()

	return out, errs
}

// Stream implements mino.RPC. TODO: errors
func (rpc RPC) Stream(ctx context.Context,
	players mino.Players) (mino.Sender, mino.Receiver, error) {

	root := newRootAddress()

	rting, err := rpc.overlay.routingFactory.FromIterator(rpc.overlay.me,
		players.AddressIterator())
	if err != nil {
		return nil, nil, xerrors.Errorf("couldn't generate routing: %v", err)
	}

	header := metadata.New(map[string]string{headerURIKey: rpc.uri})

	receiver := receiver{
		addressFactory: rpc.overlay.routingFactory.GetAddressFactory(),
		encoder:        rpc.overlay.encoder,
		errs:           make(chan error, 1),
		queue:          newNonBlockingQueue(),
	}

	gateway := rting.GetRoot()

	sender := sender{
		encoder:        rpc.overlay.encoder,
		me:             root,
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
