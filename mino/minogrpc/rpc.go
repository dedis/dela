package minogrpc

import (
	context "context"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
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

	m, err := ptypes.MarshalAny(req)
	if err != nil {
		errs <- xerrors.Errorf("failed to marshal msg to any: %v", err)
		return out, errs
	}

	sendMsg := &Message{Payload: m}

	go func() {
		iter := players.AddressIterator()
		for iter.HasNext() {
			addrStr := iter.GetNext().String()

			clientConn, err := rpc.overlay.getConnectionTo(address{addrStr})
			if err != nil {
				errs <- xerrors.Errorf("failed to get client conn for '%s': %v",
					addrStr, err)
				continue
			}
			cl := NewOverlayClient(clientConn)

			header := metadata.New(map[string]string{headerURIKey: rpc.uri})
			newCtx := metadata.NewOutgoingContext(ctx, header)

			callResp, err := cl.Call(newCtx, sendMsg)
			if err != nil {
				errs <- xerrors.Errorf("failed to call client '%s': %v", addrStr, err)
				continue
			}

			resp, err := rpc.overlay.encoder.UnmarshalDynamicAny(callResp.GetPayload())
			if err != nil {
				errs <- xerrors.Errorf("couldn't unmarshal message: %v", err)
				continue
			}

			out <- resp
		}

		close(out)
	}()

	return out, errs
}

// Stream implements mino.RPC.
func (rpc RPC) Stream(ctx context.Context,
	players mino.Players) (in mino.Sender, out mino.Receiver) {

	rting, err := rpc.overlay.routingFactory.FromIterator(orchestratorAddress, players.AddressIterator())
	if err != nil {
		panic(err)
	}

	header := metadata.New(map[string]string{headerURIKey: rpc.uri})

	sender, receiver := rpc.overlay.setupRelays(header, nil, orchestratorAddress, rting)

	return sender, receiver
}
