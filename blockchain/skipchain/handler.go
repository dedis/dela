package skipchain

import (
	fmt "fmt"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/mino"
)

type handler struct {
	mino.UnsupportedHandler

	db      Database
	factory blockFactory
	triage  *blockTriage
}

func newHandler(db Database, t *blockTriage, f blockFactory) handler {
	return handler{
		db:      db,
		factory: f,
		triage:  t,
	}
}

func (h handler) Process(req proto.Message) (proto.Message, error) {
	switch in := req.(type) {
	case *PropagateGenesis:
		genesis, err := h.factory.fromBlock(in.GetGenesis())
		if err != nil {
			return nil, err
		}

		fabric.Logger.Info().Msg("New Genesis block written")
		err = h.db.Write(genesis.(SkipBlock))
		if err != nil {
			return nil, err
		}
	case *PropagateForwardLink:
		fl, err := h.factory.fromForwardLink(in.GetLink())
		if err != nil {
			return nil, err
		}

		err = h.triage.Commit(fl)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown message type: %#v", in)
	}

	return nil, nil
}
