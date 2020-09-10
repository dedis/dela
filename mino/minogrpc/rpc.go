package minogrpc

import (
	context "context"
	"sync"

	"github.com/rs/xid"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// RPC represents an RPC that has been registered by a client, which allows
// clients to call an RPC that will execute the provided handler.
//
// - implements mino.RPC
type RPC struct {
	overlay *overlay
	uri     string
	factory serde.Factory
}

// Call implements mino.RPC. It calls the RPC on each provided address.
func (rpc *RPC) Call(ctx context.Context,
	req serde.Message, players mino.Players) (<-chan mino.Response, error) {

	data, err := req.Serialize(rpc.overlay.context)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal msg: %v", err)
	}

	from, err := rpc.overlay.me.MarshalText()
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal address: %v", err)
	}

	sendMsg := &ptypes.Message{
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

			clientConn, err := rpc.overlay.connMgr.Acquire(addr)
			if err != nil {
				resp := mino.NewResponseWithError(
					addr,
					xerrors.Errorf("failed to get client conn: %v", err),
				)

				out <- resp
				return
			}

			defer rpc.overlay.connMgr.Release(addr)

			cl := ptypes.NewOverlayClient(clientConn)

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

	streamID := xid.New().String()

	table, err := rpc.overlay.router.New(players)
	if err != nil {
		return nil, nil, xerrors.Errorf("routing table: %v", err)
	}

	md := metadata.Pairs(headerURIKey, rpc.uri, headerStreamIDKey, streamID)

	// Streamless session as this is the orchestrator of the protocol.
	sess := session.NewSession(
		md,
		orchStream{ctx: ctx},
		root,
		table,
		rpc.factory,
		rpc.overlay.router.GetPacketFactory(),
		rpc.overlay.context,
		rpc.overlay.connMgr,
	)

	rpc.overlay.closer.Add(1)

	go func() {
		defer rpc.overlay.closer.Done()

		sess.Listen()
	}()

	return sess, sess, nil
}

type orchStream struct {
	session.PacketStream

	ctx context.Context
}

func (s orchStream) Context() context.Context {
	return s.ctx
}

func (s orchStream) Recv() (*ptypes.Packet, error) {
	<-s.ctx.Done()
	return nil, status.FromContextError(s.ctx.Err()).Err()
}

func (orchStream) Send(*ptypes.Packet) error {
	return xerrors.New("no parent stream")
}
