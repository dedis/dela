package flatcosi

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

func TestHandler_Process(t *testing.T) {
	h := newHandler(bls.NewSigner(), fakeReactor{})
	req := mino.Request{
		Message: &SignatureRequest{Message: makeMessage(t)},
	}

	_, err := h.Process(req)
	require.NoError(t, err)

	resp, err := h.Process(mino.Request{Message: &empty.Empty{}})
	require.EqualError(t, err, "invalid message type: *empty.Empty")
	require.Nil(t, resp)

	h.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't unmarshal message: fake error")

	h.encoder = fake.BadPackAnyEncoder{}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't pack signature: fake error")

	h.encoder = encoding.NewProtoEncoder()
	h.reactor = fakeReactor{err: xerrors.New("oops")}
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't hash message: oops")

	h.reactor = fakeReactor{}
	h.signer = fake.NewBadSigner()
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't sign: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeReactor struct {
	err error
}

func (h fakeReactor) Invoke(mino.Address, proto.Message) ([]byte, error) {
	return []byte{0xab}, h.err
}

func makeMessage(t *testing.T) *any.Any {
	packed, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)

	return packed
}
