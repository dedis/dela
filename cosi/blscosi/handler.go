package blscosi

import (
	"errors"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
)

type handler struct {
	mino.UnsupportedHandler
	signer crypto.Signer
	hasher cosi.Hashable
}

func newHandler(s crypto.Signer, h cosi.Hashable) handler {
	return handler{
		signer: s,
	}
}

func (h handler) Process(msg proto.Message) (proto.Message, error) {
	switch req := msg.(type) {
	case *SignatureRequest:
		var da ptypes.DynamicAny
		err := ptypes.UnmarshalAny(req.Message, &da)
		if err != nil {
			return nil, err
		}

		buf, err := h.hasher.Hash(da.Message)
		if err != nil {
			return nil, err
		}

		sig, err := h.signer.Sign(buf)
		if err != nil {
			return nil, err
		}

		sigproto, err := sig.Pack()
		if err != nil {
			return nil, err
		}

		sigany, err := ptypes.MarshalAny(sigproto)
		if err != nil {
			return nil, err
		}

		return &SignatureResponse{Signature: sigany}, nil
	}

	return nil, errors.New("unknown type of message")
}
