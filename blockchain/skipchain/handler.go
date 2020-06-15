package skipchain

import (
	"context"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/tmp"
	"golang.org/x/xerrors"
)

// handler is the RPC handler. The only message processed is the genesis block
// propagation.
//
// - implements mino.Handler
type handler struct {
	mino.UnsupportedHandler
	*operations

	requestFactory   serde.Factory
	propagateFactory serde.Factory
}

func newHandler(ops *operations) handler {
	return handler{
		operations:       ops,
		requestFactory:   requestFactory{},
		propagateFactory: propagateFactory{blockFactory: ops.blockFactory},
	}
}

// Process implements mino.Handler. It handles genesis block propagation
// messages only and return an error for any other type.
func (h handler) Process(req mino.Request) (proto.Message, error) {
	in := tmp.FromProto(req.Message, h.propagateFactory)

	switch msg := in.(type) {
	case PropagateGenesis:
		err := h.commitBlock(msg.genesis)
		if err != nil {
			return nil, xerrors.Errorf("couldn't store genesis: %v", err)
		}

		return nil, nil
	default:
		return nil, xerrors.Errorf("unknown message type '%T'", in)
	}
}

// Stream implements mino.Handler. It handles block requests to help another
// participant to catch up the latest chain.
func (h handler) Stream(out mino.Sender, in mino.Receiver) error {
	addr, msg, err := in.Recv(context.Background())
	if err != nil {
		return xerrors.Errorf("couldn't receive message: %v", err)
	}

	m := tmp.FromProto(msg, h.requestFactory)

	req, ok := m.(BlockRequest)
	if !ok {
		return xerrors.Errorf("invalid message type '%T' != '%T'", m, req)
	}

	for i := req.from; i <= req.to; i++ {
		block, err := h.db.Read(int64(i))
		if err != nil {
			return xerrors.Errorf("couldn't read block at index %d: %v", i, err)
		}

		resp := BlockResponse{
			block: block,
		}

		err = <-out.Send(tmp.ProtoOf(resp), addr)
		if err != nil {
			return xerrors.Errorf("couldn't send block: %v", err)
		}
	}

	return nil
}
