package flatcosi

import (
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

type handler struct {
	mino.UnsupportedHandler
	signer  crypto.Signer
	reactor cosi.Reactor
}

func newHandler(s crypto.Signer, r cosi.Reactor) handler {
	return handler{
		signer:  s,
		reactor: r,
	}
}

func (h handler) Process(req mino.Request) (serdeng.Message, error) {
	switch msg := req.Message.(type) {
	case cosi.SignatureRequest:
		buf, err := h.reactor.Invoke(req.Address, msg.Value)
		if err != nil {
			return nil, xerrors.Errorf("couldn't hash message: %v", err)
		}

		sig, err := h.signer.Sign(buf)
		if err != nil {
			return nil, xerrors.Errorf("couldn't sign: %v", err)
		}

		resp := cosi.SignatureResponse{
			Signature: sig,
		}

		return resp, nil
	default:
		return nil, xerrors.Errorf("invalid message type '%T'", msg)
	}
}
