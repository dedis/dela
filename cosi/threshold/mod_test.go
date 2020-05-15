package threshold

import (
	"context"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/crypto/bls"
	"go.dedis.ch/fabric/encoding"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minoch"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&SignatureProto{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestCoSi_Basic(t *testing.T) {
	manager := minoch.NewManager()

	m1, err := minoch.NewMinoch(manager, "A")
	require.NoError(t, err)

	m2, err := minoch.NewMinoch(manager, "B")
	require.NoError(t, err)

	ca := fake.NewAuthorityFromMino(bls.NewSigner, m1, m2)
	c1 := NewCoSi(m1, ca.GetSigner(0))
	c1.Threshold = func(n int) int { return n - 1 }

	actor, err := c1.Listen(fakeHashable{})
	require.NoError(t, err)

	c2 := NewCoSi(m2, ca.GetSigner(1))
	_, err = c2.Listen(fakeHashable{err: xerrors.New("oops")})
	require.NoError(t, err)

	ctx := context.Background()
	sig, err := actor.Sign(ctx, fakeMessage{}, ca)
	require.NoError(t, err)
	require.NotNil(t, sig)

	verifier, err := c1.GetVerifierFactory().FromAuthority(ca)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify([]byte{0xff}, sig))
}

func TestDefaultThreshold(t *testing.T) {
	require.Equal(t, 2, defaultThreshold(2))
	require.Equal(t, 5, defaultThreshold(5))
}

func TestCoSi_GetSigner(t *testing.T) {
	c := &CoSi{signer: fake.NewSigner()}
	require.NotNil(t, c.GetSigner())
}

func TestCoSi_GetPublicKeyFactory(t *testing.T) {
	c := &CoSi{signer: fake.NewSigner()}
	require.NotNil(t, c.GetPublicKeyFactory())
}

func TestCoSi_GetSignatureFactory(t *testing.T) {
	c := &CoSi{signer: fake.NewSigner()}
	require.NotNil(t, c.GetSignatureFactory())
}

func TestCoSi_Listen(t *testing.T) {
	c := &CoSi{mino: fake.Mino{}}

	actor, err := c.Listen(fakeHashable{})
	require.NoError(t, err)
	require.NotNil(t, actor)

	c.mino = fake.NewBadMino()
	_, err = c.Listen(fakeHashable{})
	require.EqualError(t, err, "couldn't make rpc: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeHashable struct {
	cosi.Hashable
	err error
}

func (h fakeHashable) Hash(addr mino.Address, in proto.Message) ([]byte, error) {
	return []byte{0xff}, h.err
}

type fakeMessage struct {
	cosi.Message
}

func (m fakeMessage) GetHash() []byte {
	return []byte{0xff}
}

func (m fakeMessage) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &wrappers.StringValue{Value: "abc"}, nil
}
