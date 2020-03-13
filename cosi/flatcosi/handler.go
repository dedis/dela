package flatcosi

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

type handler struct {
	mino.UnsupportedHandler
	signer crypto.Signer
	hasher cosi.Hashable
}

func newHandler(s crypto.Signer, h cosi.Hashable) handler {
	return handler{
		signer: s,
		hasher: h,
	}
}

func (h handler) Process(msg proto.Message) (proto.Message, error) {
	var resp proto.Message

	switch req := msg.(type) {
	case *SignatureRequest:
		var da ptypes.DynamicAny
		err := protoenc.UnmarshalAny(req.Message, &da)
		if err != nil {
			return nil, encoding.NewAnyDecodingError(&da, err)
		}

		buf, err := h.hasher.Hash(da.Message)
		if err != nil {
			return nil, xerrors.Errorf("couldn't hash message: %v", err)
		}

		sig, err := h.signer.Sign(buf)
		if err != nil {
			return nil, xerrors.Errorf("couldn't sign: %v", err)
		}

		sigproto, err := sig.Pack()
		if err != nil {
			return nil, encoding.NewEncodingError("signature", err)
		}

		sigany, err := protoenc.MarshalAny(sigproto)
		if err != nil {
			return nil, encoding.NewAnyEncodingError(sigproto, err)
		}

		resp = &SignatureResponse{Signature: sigany}
	default:
		return nil, xerrors.Errorf("invalid message type: %T", msg)
	}

	return resp, nil
}
