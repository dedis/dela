package types

import (
	"bytes"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/internal/testing/fake"
)

func init() {
	RegisterRequestFormat(fake.GoodFormat, fake.Format{Msg: Prepare{}})
	RegisterRequestFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestPrepare_GetMessage(t *testing.T) {
	p := NewPrepare(fake.Message{}, nil, nil)

	require.Equal(t, fake.Message{}, p.GetMessage())
}

func TestPrepare_GetSignature(t *testing.T) {
	p := NewPrepare(nil, fake.Signature{}, nil)

	require.Equal(t, fake.Signature{}, p.GetSignature())
}

func TestPrepare_GetChain(t *testing.T) {
	p := NewPrepare(nil, nil, fakeChain{})

	require.Equal(t, fakeChain{}, p.GetChain())
}

func TestPrepare_Serialize(t *testing.T) {
	p := Prepare{}

	data, err := p.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = p.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode prepare: fake error")
}

func TestCommit_Getters(t *testing.T) {
	f := func(to []byte) bool {
		c := NewCommit(to, fake.Signature{})

		require.Equal(t, fake.Signature{}, c.GetPrepare())

		return bytes.Equal(to, c.GetTo())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestCommit_Serialize(t *testing.T) {
	c := Commit{}

	data, err := c.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = c.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode commit: fake error")
}

func TestPropagate_Getters(t *testing.T) {
	f := func(to []byte) bool {
		p := NewPropagate(to, fake.Signature{})

		require.Equal(t, fake.Signature{}, p.GetCommit())

		return bytes.Equal(to, p.GetTo())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestPropagate_Serialize(t *testing.T) {
	p := Propagate{}

	data, err := p.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = p.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode propagate: fake error")
}

func TestRequestFactory_Deserialize(t *testing.T) {
	factory := NewRequestFactory(
		fake.MessageFactory{},
		fake.SignatureFactory{},
		fake.SignatureFactory{},
		fakeChainFactory{})

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, Prepare{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode request: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeChain struct {
	consensus.Chain
}

type fakeChainFactory struct {
	consensus.ChainFactory
}
