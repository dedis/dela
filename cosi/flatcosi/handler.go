package flatcosi

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/tmp"
	"golang.org/x/xerrors"
)

type handler struct {
	mino.UnsupportedHandler
	signer  crypto.Signer
	reactor cosi.Reactor
	factory serde.Factory
}

func newHandler(s crypto.Signer, r cosi.Reactor) handler {
	return handler{
		signer:  s,
		reactor: r,
		factory: newRequestFactory(r),
	}
}

func (h handler) Process(req mino.Request) (proto.Message, error) {
	in := tmp.FromProto(req.Message, h.factory)

	switch msg := in.(type) {
	case SignatureRequest:
		buf, err := h.reactor.Invoke(req.Address, msg.message)
		if err != nil {
			return nil, xerrors.Errorf("couldn't hash message: %v", err)
		}

		sig, err := h.signer.Sign(buf)
		if err != nil {
			return nil, xerrors.Errorf("couldn't sign: %v", err)
		}

		resp := SignatureResponse{
			signature: sig,
		}

		return tmp.ProtoOf(resp), nil
	default:
		return nil, xerrors.Errorf("invalid message type '%T'", msg)
	}
}
