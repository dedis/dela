package flatcosi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/tmp"
)

func TestHandler_Process(t *testing.T) {
	h := newHandler(bls.NewSigner(), fakeReactor{})
	req := mino.Request{
		Message: tmp.ProtoOf(SignatureRequest{message: fake.Message{}}),
	}

	_, err := h.Process(req)
	require.NoError(t, err)

	h.factory = fake.MessageFactory{}
	resp, err := h.Process(mino.Request{Message: tmp.ProtoOf(fake.Message{})})
	require.EqualError(t, err, "invalid message type 'fake.Message'")
	require.Nil(t, resp)

	h.factory = newRequestFactory(h.reactor)
	h.signer = fake.NewBadSigner()
	_, err = h.Process(req)
	require.EqualError(t, err, "couldn't sign: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeReactor struct {
	serde.UnimplementedFactory

	err error
}

func (h fakeReactor) Invoke(mino.Address, serde.Message) ([]byte, error) {
	return []byte{0xab}, h.err
}

func (h fakeReactor) VisitJSON(serde.FactoryInput) (serde.Message, error) {
	return fake.Message{}, nil
}
