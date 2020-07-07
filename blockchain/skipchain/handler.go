package skipchain

import (
	"context"

	"go.dedis.ch/dela/blockchain/skipchain/types"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
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
func (h handler) Process(req mino.Request) (serde.Message, error) {
	switch msg := req.Message.(type) {
	case types.PropagateGenesis:
		err := h.commitBlock(msg.GetGenesis())
		if err != nil {
			return nil, xerrors.Errorf("couldn't store genesis: %v", err)
		}

		return nil, nil
	default:
		return nil, xerrors.Errorf("unknown message type '%T'", req.Message)
	}
}

// Stream implements mino.Handler. It handles block requests to help another
// participant to catch up the latest chain.
func (h handler) Stream(out mino.Sender, in mino.Receiver) error {
	addr, msg, err := in.Recv(context.Background())
	if err != nil {
		return xerrors.Errorf("couldn't receive message: %v", err)
	}

	req, ok := msg.(types.BlockRequest)
	if !ok {
		return xerrors.Errorf("invalid message type '%T' != '%T'", msg, req)
	}

	for i := req.GetFrom(); i <= req.GetTo(); i++ {
		block, err := h.db.Read(int64(i))
		if err != nil {
			return xerrors.Errorf("couldn't read block at index %d: %v", i, err)
		}

		resp := types.NewBlockResponse(block)

		err = <-out.Send(resp, addr)
		if err != nil {
			return xerrors.Errorf("couldn't send block: %v", err)
		}
	}

	return nil
}
