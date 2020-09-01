package minogrpc

import (
	context "context"
	"crypto/rand"
	"sync"

	"github.com/rs/xid"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/metadata"
)

const streamIDLen = 16

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

			if callResp.GetPayload() == nil {
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

	b := make([]byte, streamIDLen)
	_, err := rand.Reader.Read(b)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to create streamID: %v", err)
	}

	streamID := xid.New().String()

	receiver := receiver{
		context:        rpc.overlay.context,
		factory:        rpc.factory,
		addressFactory: rpc.overlay.addrFactory,
		errs:           make(chan error, 1),
		queue:          newNonBlockingQueue(),

		logger: dela.Logger.With().Str("addr", root.String()).Logger().
			With().Str("streamID", streamID).Logger(),
	}

	sender := sender{
		me: root,
		// There is no gateway because this is the root
		clients:  map[mino.Address]chan OutContext{},
		receiver: receiver,
		traffic:  rpc.overlay.traffic,

		router:      rpc.overlay.router,
		connFactory: rpc.overlay.connFactory,
		uri:         rpc.uri,

		streamID: streamID,
		lock:     new(sync.Mutex),
		done:     make(chan struct{}),
	}

	go func() {
		<-ctx.Done()
		close(sender.done)
	}()

	return sender, receiver, nil
}
