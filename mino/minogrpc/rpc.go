package minogrpc

import (
	"context"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// GrpcRPC ...
type GrpcRPC struct {
}

// Call ...
func (rpc GrpcRPC) Call(req proto.Message,
	addrs ...*mino.Address) (<-chan proto.Message, <-chan error) {

	return nil, nil
}

// Stream ...
func (rpc GrpcRPC) Stream(ctx context.Context,
	addrs ...*mino.Address) (in mino.Sender, out mino.Receiver) {

	return nil, nil
}

// Sender ...
type Sender struct {
	overlay OverlayServer
}

// Send sends msg to addrs, which should call the Receiver.Recv of each addrs.
func (s Sender) Send(msg proto.Message, addrs ...*mino.Address) error {
	m, err := ptypes.MarshalAny(msg)
	if err != nil {
		return xerrors.Errorf("failed to marshal msg to any: %v", err)
	}

	sendMsg := &SendMessage{
		Message: m,
	}
	_, err = s.overlay.Send(nil, sendMsg)
	if err != nil {
		return xerrors.Errorf("failed to send message: %v", err)
	}

	return nil
}

// Receiver ...
type Receiver struct {
	overlay OverlayClient
}

// Recv ...
func (r Receiver) Recv(ctx context.Context) (*mino.Address, proto.Message, error) {
	return nil, nil, nil
}
