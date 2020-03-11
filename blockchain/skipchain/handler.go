package skipchain

import (
	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

type handler struct {
	mino.UnsupportedHandler

	db               Database
	blockFactory     *blockFactory
	publicKeyFactory crypto.PublicKeyFactory
}

func newHandler(db Database, f *blockFactory) handler {
	return handler{
		db:               db,
		blockFactory:     f,
		publicKeyFactory: f.cosi.GetPublicKeyFactory(),
	}
}

func (h handler) Process(req proto.Message) (proto.Message, error) {
	switch in := req.(type) {
	case *PropagateGenesis:
		genesis, err := h.blockFactory.decodeBlock(h.publicKeyFactory, in.GetGenesis())
		if err != nil {
			return nil, xerrors.Errorf("couldn't decode the block: %v", err)
		}

		fabric.Logger.Debug().Msgf("New Genesis block written: %v", genesis.hash)
		err = h.db.Write(genesis)
		if err != nil {
			return nil, xerrors.Errorf("couldn't write the block: %v", err)
		}
	default:
		return nil, xerrors.Errorf("unknown message type: %#v", in)
	}

	return nil, nil
}
