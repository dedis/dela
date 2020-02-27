package skipchain

import (
	"errors"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/m"
	"go.dedis.ch/m/mino"
)

type handler struct {
	mino.UnsupportedHandler

	db      Database
	factory blockFactory
}

func newHandler(db Database, f blockFactory) handler {
	return handler{
		db:      db,
		factory: f,
	}
}

func (h handler) Process(req proto.Message) (proto.Message, error) {
	switch in := req.(type) {
	case *PropagateGenesis:
		genesis, err := h.factory.fromBlock(in.GetGenesis())
		if err != nil {
			return nil, err
		}

		m.Logger.Info().Msg("New Genesis block written")
		err = h.db.Write(genesis.(SkipBlock))
		if err != nil {
			return nil, err
		}

		return nil, nil
	}

	return nil, errors.New("unknown message type")
}
