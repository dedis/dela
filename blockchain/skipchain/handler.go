package skipchain

import (
	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
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
}

func newHandler(sc *Skipchain) handler {
	return handler{
		Skipchain: sc,
	}
}

// Process implements mino.Handler. It handles genesis block propagation
// messages only and return an error for any other type.
func (h handler) Process(req proto.Message) (proto.Message, error) {
	switch in := req.(type) {
	case *PropagateGenesis:
		factory := h.GetBlockFactory().(blockFactory)

		genesis, err := factory.decodeBlock(in.GetGenesis())
		if err != nil {
			return nil, xerrors.Errorf("couldn't decode the block: %v", err)
		}

		err = h.db.Write(genesis)
		if err != nil {
			return nil, xerrors.Errorf("couldn't write the block: %v", err)
		}

		fabric.Logger.Trace().Msgf("New Genesis block written: %v", genesis.hash)
		return nil, nil
	default:
		return nil, xerrors.Errorf("unknown message type '%T'", in)
	}
}
