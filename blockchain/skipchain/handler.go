package skipchain

import (
	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

type handler struct {
	mino.UnsupportedHandler

	db      Database
	factory *blockFactory
	triage  *blockTriage
}

func newHandler(db Database, t *blockTriage, f *blockFactory) handler {
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
			return nil, xerrors.Errorf("couldn't decode the block: %v", err)
		}

		fabric.Logger.Info().Msg("New Genesis block written")
		err = h.db.Write(genesis.(SkipBlock))
		if err != nil {
			return nil, xerrors.Errorf("couldn't write the block: %v", err)
		}
	case *PropagateForwardLink:
		fl, err := h.factory.fromForwardLink(in.GetLink())
		if err != nil {
			return nil, xerrors.Errorf("couldn't decode the forward link: %v", err)
		}

		err = h.triage.Commit(fl.(forwardLink))
		if err != nil {
			return nil, xerrors.Errorf("couldn't commit the forward link: %v", err)
		}
	default:
		return nil, xerrors.Errorf("unknown message type: %#v", in)
	}

	return nil, nil
}
