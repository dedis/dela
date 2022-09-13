// This file contains the implementation of the RPC in the context of gRPC.
//
// Documentation Last Review: 07.10.2020
//

package minogrpc

import (
	context "context"
	"github.com/rs/xid"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/internal/tracing"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"sync"
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
		return nil, xerrors.Errorf("while serializing: %v", err)
	}

	sendMsg := &ptypes.Message{
		From:    []byte(rpc.overlay.myAddrStr),
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

// Stream implements mino.RPC. It will open a stream to one of the addresses
// with a bidirectional channel that will send and receive packets. The chosen
// address will open one or several streams to the rest of the players. The
// choice of the gateway is first the local node if it belongs to the list,
// otherwise the first node of the list.
//
// The way routes are created depends on the router implementation chosen for
// the endpoint. It can for instance use a tree structure, which means the
// network for 8 nodes could look like this:
//
//	  Orchestrator
//	        |
//	     __ A __
//	    /       \
//	   B         C
//	 / | \     /   \
//	D  E  F   G     H
//
// If C has to send a message to B, it will send it through node A. Similarly,
// if D has to send a message to G, it will move up the tree through B, A and
// finally C.
func (rpc RPC) Stream(ctx context.Context, players mino.Players) (mino.Sender, mino.Receiver, error) {
	if players == nil || players.Len() == 0 {
		return nil, nil, xerrors.New("empty list of addresses")
	}

	streamID := xid.New().String()

	md := metadata.Pairs(
		headerURIKey, rpc.uri,
		headerStreamIDKey, streamID,
		headerGatewayKey, rpc.overlay.myAddrStr,
	)

	protocol := ctx.Value(tracing.ProtocolKey)

	if protocol != nil {
		md.Append(tracing.ProtocolTag, protocol.(string))
	} else {
		md.Append(tracing.ProtocolTag, tracing.UndefinedProtocol)
	}

	table, err := rpc.overlay.router.New(mino.NewAddresses(), rpc.overlay.myAddr)
	if err != nil {
		return nil, nil, xerrors.Errorf("routing table failed: %v", err)
	}

	gw, others := rpc.findGateway(players)

	for _, addr := range others {
		addr, err := addr.MarshalText()
		if err != nil {
			return nil, nil, xerrors.Errorf("while marshaling address: %v", err)
		}

		md.Append(headerAddressKey, string(addr))
	}

	conn, err := rpc.overlay.connMgr.Acquire(gw)
	if err != nil {
		return nil, nil, xerrors.Errorf("gateway connection failed: %v", err)
	}

	client := ptypes.NewOverlayClient(conn)

	ctx = metadata.NewOutgoingContext(ctx, md)

	stream, err := client.Stream(ctx)
	if err != nil {
		rpc.overlay.connMgr.Release(gw)

		return nil, nil, xerrors.Errorf("failed to open stream: %v", err)
	}

	// Wait for the event from the server to tell that the stream is
	// initialized.
	_, err = stream.Header()
	if err != nil {
		rpc.overlay.connMgr.Release(gw)

		return nil, nil, xerrors.Errorf("failed to receive header: %v", err)
	}

	relay := session.NewRelay(stream, gw, rpc.overlay.context, conn, md)

	sess := session.NewSession(
		md,
		session.NewOrchestratorAddress(rpc.overlay.myAddr),
		rpc.factory,
		rpc.overlay.router.GetPacketFactory(),
		rpc.overlay.context,
		rpc.overlay.connMgr,
	)

	// There is no listen for the orchestrator as we need to forward the
	// messages received from the stream.
	sess.SetPassive(relay, table)

	rpc.overlay.closer.Add(1)

	go func() {
		defer func() {
			relay.Close()
			rpc.overlay.connMgr.Release(gw)
			rpc.overlay.closer.Done()
		}()

		for {
			p, err := stream.Recv()
			if err != nil {
				if status.Code(err) == codes.Unknown {
					dela.Logger.Err(err).Msg("stream to root failed")
				}

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

		if addr.Equal(rpc.overlay.myAddr) {
			gw = addr
		} else {
			addrs = append(addrs, addr)
		}
	}

	if gw == nil {
		return addrs[0], addrs[1:]
	}

	return gw, addrs
}
