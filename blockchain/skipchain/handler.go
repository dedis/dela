package skipchain

import (
	"bytes"
	"context"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// handler is the RPC handler. The only message processed is the genesis block
// propagation.
//
// - implements mino.Handler
type handler struct {
	mino.UnsupportedHandler
	*operations
}

func newHandler(ops *operations) handler {
	return handler{
		operations: ops,
	}
}

// Process implements mino.Handler. It handles genesis block propagation
// messages only and return an error for any other type.
func (h handler) Process(req mino.Request) (proto.Message, error) {
	switch in := req.Message.(type) {
	case *PropagateGenesis:
		_, err := h.insertBlock(in.GetGenesis())
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

	req, ok := msg.(*BlockRequest)
	if !ok {
		return xerrors.Errorf("invalid message type '%T' != '%T'", msg, req)
	}

	var block SkipBlock
	for i := int64(0); !bytes.Equal(block.hash[:], req.To); i++ {
		block, err = h.db.Read(i)
		if err != nil {
			return xerrors.Errorf("couldn't read block at index %d: %v", i, err)
		}

		blockpb, err := h.encoder.Pack(block)
		if err != nil {
			return xerrors.Errorf("couldn't pack block: %v", err)
		}

		errs := out.Send(&BlockResponse{Block: blockpb.(*BlockProto)}, addr)

		err = <-errs
		if err != nil {
			return xerrors.Errorf("couldn't send block: %v", err)
		}
	}

	return nil
}
