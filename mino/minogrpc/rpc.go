package minogrpc

import (
	context "context"
	"io"
	"sync"

	"github.com/rs/xid"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/metadata"
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

// Stream implements mino.RPC.
func (rpc RPC) Stream(ctx context.Context, players mino.Players) (mino.Sender, mino.Receiver, error) {
	streamID := xid.New().String()

	me, err := newRootAddress().MarshalText()
	if err != nil {
		return nil, nil, err
	}

	md := metadata.Pairs(
		headerURIKey, rpc.uri,
		headerStreamIDKey, streamID,
		headerGateway, string(me))

	table, err := rpc.overlay.router.New(mino.NewAddresses())
	if err != nil {
		return nil, nil, err
	}

	gw, others := rpc.findGateway(players)

	for _, addr := range others {
		addr, err := addr.MarshalText()
		if err != nil {
			return nil, nil, err
		}

		md.Append("addr", string(addr))
	}

	conn, err := rpc.overlay.connMgr.Acquire(gw)
	if err != nil {
		return nil, nil, err
	}

	client := ptypes.NewOverlayClient(conn)

	ctx = metadata.NewOutgoingContext(ctx, md)

	stream, err := client.Stream(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Wait for the event from the server to tell that the stream is
	// initialized.
	_, err = stream.Header()
	if err != nil {
		return nil, nil, err
	}

	relay, err := session.NewRelay(stream, gw, rpc.overlay.context, rpc.overlay.connMgr, md)
	if err != nil {
		return nil, nil, err
	}

	sess := session.NewSession(
		md,
		relay,
		newRootAddress(),
		table,
		rpc.factory,
		rpc.overlay.router.GetPacketFactory(),
		rpc.overlay.context,
		rpc.overlay.connMgr,
	)

	go func() {
		defer relay.Close()

		for {
			p, err := stream.Recv()
			if err == io.EOF {
				return
			}

			sess.RecvPacket(gw, p)
		}
	}()

	return sess, sess, nil
}

func (rpc RPC) findGateway(players mino.Players) (mino.Address, []mino.Address) {
	iter := players.AddressIterator()
	addrs := make([]mino.Address, 0, players.Len())

	var gw mino.Address

	for iter.HasNext() {
		addr := iter.GetNext()

		if !addr.Equal(rpc.overlay.me) {
			addrs = append(addrs, addr)
		} else {
			gw = addr
		}
	}

	if gw == nil {
		return addrs[0], addrs[1:]
	}

	return gw, addrs
}
