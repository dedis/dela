package threshold

import (
	"context"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/encoding"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minoch"
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

	ca := fake.NewCollectiveAuthorityFromMino(m1)
	c1 := NewCoSi(m1, ca.GetSigner(0))

	actor, err := c1.Listen(fakeHashable{})
	require.NoError(t, err)

	ctx := context.Background()
	sig, err := actor.Sign(ctx, fakeMessage{}, ca)
	require.NoError(t, err)
	require.NotNil(t, sig)

	verifier, err := c1.GetVerifier(ca)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify([]byte{0xff}, sig))
}

func TestCoSi_GetPublicKeyFactory(t *testing.T) {
	c := &CoSi{signer: fake.NewSigner()}
	require.NotNil(t, c.GetPublicKeyFactory())
}

func TestCoSi_GetSignatureFactory(t *testing.T) {
	c := &CoSi{signer: fake.NewSigner()}
	require.NotNil(t, c.GetSignatureFactory())
}

func TestCoSi_GetVerifier(t *testing.T) {
	c := &CoSi{signer: fake.NewSigner()}

	verifier, err := c.GetVerifier(fake.NewCollectiveAuthority(3))
	require.NoError(t, err)
	require.NotNil(t, verifier)
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

func (m fakeMessage) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &wrappers.StringValue{Value: "abc"}, nil
}
