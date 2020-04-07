package flatcosi

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/crypto/bls"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

type fakeHasher struct {
	cosi.Hashable
	err error
}

func (h fakeHasher) Hash(mino.Address, proto.Message) ([]byte, error) {
	return []byte{0xab}, h.err
}

func TestHandler_Process(t *testing.T) {
	h := newHandler(bls.NewSigner(), fakeHasher{})
	req := mino.Request{
		Message: &SignatureRequest{Message: makeMessage(t)},
	}

	_, err := h.Process(req)
	require.NoError(t, err)

	resp, err := h.Process(mino.Request{Message: &empty.Empty{}})
	require.EqualError(t, err, "invalid message type: *empty.Empty")
	require.Nil(t, resp)

	h.encoder = badUnmarshalDynEncoder{}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't unmarshal message: oops")

	h.encoder = badPackAnyEncoder{}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't pack signature: oops")

	h.encoder = encoding.NewProtoEncoder()
	h.hasher = fakeHasher{err: xerrors.New("oops")}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't hash message: oops")

	h.hasher = fakeHasher{}
	h.signer = fakeSigner{err: xerrors.New("oops")}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't sign: oops")
}

func makeMessage(t *testing.T) *any.Any {
	packed, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)

	return packed
}
