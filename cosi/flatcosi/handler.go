package flatcosi

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

type handler struct {
	mino.UnsupportedHandler
	signer  crypto.Signer
	reactor cosi.Reactor
	encoder encoding.ProtoMarshaler
}

func newHandler(s crypto.Signer, r cosi.Reactor) handler {
	return handler{
		signer:  s,
		reactor: r,
		encoder: encoding.NewProtoEncoder(),
	}
}

func (h handler) Process(req mino.Request) (proto.Message, error) {
	switch msg := req.Message.(type) {
	case *SignatureRequest:
		data, err := h.encoder.UnmarshalDynamicAny(msg.Message)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal message: %v", err)
		}

		buf, err := h.reactor.Invoke(req.Address, data)
		if err != nil {
			return nil, xerrors.Errorf("couldn't hash message: %v", err)
		}

		sig, err := h.signer.Sign(buf)
		if err != nil {
			return nil, xerrors.Errorf("couldn't sign: %v", err)
		}

		resp := &SignatureResponse{}
		resp.Signature, err = h.encoder.PackAny(sig)
		if err != nil {
			return nil, xerrors.Errorf("couldn't pack signature: %v", err)
		}

		return resp, nil
	default:
		return nil, xerrors.Errorf("invalid message type: %T", msg)
	}
}
