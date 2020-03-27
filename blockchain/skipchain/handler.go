package skipchain

import (
	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// handler is the RPC handler. The only message processed is the genesis block
// propagation.
//
// - implements mino.Handler
type handler struct {
	mino.UnsupportedHandler
	*Skipchain

	proc blockchain.PayloadProcessor
}

func newHandler(sc *Skipchain, proc blockchain.PayloadProcessor) handler {
	return handler{
		Skipchain: sc,
		proc:      proc,
	}
}

// Process implements mino.Handler. It handles genesis block propagation
// messages only and return an error for any other type.
func (h handler) Process(req mino.Request) (proto.Message, error) {
	switch in := req.Message.(type) {
	case *PropagateGenesis:
		factory := h.GetBlockFactory().(blockFactory)

		genesis, err := factory.decodeBlock(in.GetGenesis())
		if err != nil {
			return nil, xerrors.Errorf("couldn't decode the block: %v", err)
		}

		err = h.proc.Validate(0, genesis.GetPayload())
		if err != nil {
			return nil, xerrors.Errorf("couldn't validate genesis payload: %v", err)
		}

		err = h.db.Atomic(func(ops Queries) error {
			err = ops.Write(genesis)
			if err != nil {
				return xerrors.Errorf("couldn't write the block: %v", err)
			}

			err = h.proc.Commit(genesis.GetPayload())
			if err != nil {
				return xerrors.Errorf("couldn't commit genesis payload: %v", err)
			}

			return nil
		})
		if err != nil {
			return nil, xerrors.Errorf("tx aborted: %v", err)
		}

		fabric.Logger.Trace().Msgf("new genesis block written: %v", genesis.hash)
		return nil, nil
	default:
		return nil, xerrors.Errorf("unknown message type '%T'", in)
	}
}
