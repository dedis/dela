package types

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/internal/testing/fake"
)

func init() {
	RegisterMessageFormat(fake.GoodFormat, fake.Format{Msg: GenesisMessage{}})
	RegisterMessageFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestGenesisMessage_GetGenesis(t *testing.T) {
	msg := NewGenesisMessage(Genesis{})

	require.NotNil(t, msg.GetGenesis())
}

func TestGenesisMessage_Serialize(t *testing.T) {
	msg := NewGenesisMessage(Genesis{})

	data, err := msg.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = msg.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "encoding failed: fake error")
}

func TestBlockMessage_GetBlock(t *testing.T) {
	expected := Block{index: 1}
	msg := NewBlockMessage(expected)

	block := msg.GetBlock()
	require.Equal(t, expected, block)
}

func TestBlockMessage_Serialize(t *testing.T) {
	msg := NewBlockMessage(Block{})

	data, err := msg.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = msg.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "encoding failed: fake error")
}

func TestCommitMessage_GetID(t *testing.T) {
	msg := NewCommit(Digest{1}, fake.Signature{})

	require.Equal(t, Digest{1}, msg.GetID())
}

func TestCommitMessage_GetSignature(t *testing.T) {
	msg := NewCommit(Digest{}, fake.Signature{})

	require.Equal(t, fake.Signature{}, msg.GetSignature())
}

func TestCommitMessage_Serialize(t *testing.T) {
	msg := NewCommit(Digest{}, fake.Signature{})

	data, err := msg.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = msg.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "encoding failed: fake error")
}

func TestDoneMessage_GetID(t *testing.T) {
	msg := NewDone(Digest{1}, fake.Signature{})

	require.Equal(t, Digest{1}, msg.GetID())
}

func TestDoneMessage_GetSignature(t *testing.T) {
	msg := NewDone(Digest{}, fake.Signature{})

	require.Equal(t, fake.Signature{}, msg.GetSignature())
}

func TestDoneMessage_Serialize(t *testing.T) {
	msg := NewDone(Digest{}, fake.Signature{})

	data, err := msg.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = msg.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "encoding failed: fake error")
}

func TestViewMessage_GetID(t *testing.T) {
	msg := NewViewMessage(Digest{1}, 0)

	require.Equal(t, Digest{1}, msg.GetID())
}

func TestViewMessage_GetLeader(t *testing.T) {
	msg := NewViewMessage(Digest{}, 2)

	require.Equal(t, 2, msg.GetLeader())
}

func TestViewMessage_Serialize(t *testing.T) {
	msg := NewViewMessage(Digest{}, 3)

	data, err := msg.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = msg.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "encoding failed: fake error")
}

func TestMessageFactory_Deserialize(t *testing.T) {
	fac := NewMessageFactory(GenesisFactory{}, BlockFactory{}, fake.SignatureFactory{}, authority.NewChangeSetFactory(fake.AddressFactory{}, fake.PublicKeyFactory{}))

	msg, err := fac.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, GenesisMessage{}, msg)

	_, err = fac.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "decoding failed: fake error")
}
