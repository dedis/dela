package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

func TestChangeSetFormat_Encode(t *testing.T) {
	changeset := roster.ChangeSet{
		Remove: []uint32{42},
		Add: []roster.Player{
			{
				Address:   fake.NewAddress(2),
				PublicKey: fake.PublicKey{},
			},
		},
	}

	format := changeSetFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, changeset)
	require.NoError(t, err)
	expected := `{"Remove":[42],"Add":[{"Address":"AgAAAA==","PublicKey":{}}]}`
	require.Equal(t, expected, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")

	_, err = format.Encode(fake.NewBadContext(), changeset)
	require.EqualError(t, err, "couldn't marshal: fake error")

	changeset.Add[0].PublicKey = fake.NewBadPublicKey()
	_, err = format.Encode(ctx, changeset)
	require.EqualError(t, err, "couldn't serialize public key: fake error")

	changeset.Add[0].Address = fake.NewBadAddress()
	_, err = format.Encode(ctx, changeset)
	require.EqualError(t, err, "couldn't serialize address: fake error")
}

func TestChangeSetFormat_Decode(t *testing.T) {
	format := changeSetFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})
	ctx = serde.WithFactory(ctx, roster.AddrKeyFac{}, fake.AddressFactory{})
	ctx = serde.WithFactory(ctx, roster.PubKeyFac{}, fake.PublicKeyFactory{})

	cset, err := format.Decode(ctx, []byte(`{"Add":[{}]}`))
	require.NoError(t, err)
	player := roster.Player{Address: fake.NewAddress(0), PublicKey: fake.PublicKey{}}
	require.Equal(t, roster.ChangeSet{Add: []roster.Player{player}}, cset)

	cset, err = format.Decode(ctx, []byte(`{"Add":[]}`))
	require.NoError(t, err)
	require.Equal(t, roster.ChangeSet{}, cset)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize change set: fake error")

	badCtx := serde.WithFactory(ctx, roster.PubKeyFac{}, fake.NewBadPublicKeyFactory())
	_, err = format.Decode(badCtx, []byte(`{"Add":[{}]}`))
	require.EqualError(t, err, "couldn't deserialize public key: fake error")

	badCtx = serde.WithFactory(ctx, roster.AddrKeyFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Add":[{}]}`))
	require.EqualError(t, err, "invalid address factory of type '<nil>'")

	badCtx = serde.WithFactory(ctx, roster.PubKeyFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Add":[{}]}`))
	require.EqualError(t, err, "invalid public key factory of type '<nil>'")
}

func TestRosterFormat_Encode(t *testing.T) {
	ro := roster.FromAuthority(fake.NewAuthority(1, fake.NewSigner))

	format := rosterFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, ro)
	require.NoError(t, err)
	require.Equal(t, `[{"Address":"AAAAAA==","PublicKey":{}}]`, string(data))

	_, err = format.Encode(fake.NewContext(), fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")

	_, err = format.Encode(fake.NewBadContext(), ro)
	require.EqualError(t, err, "couldn't marshal: fake error")

	ro = roster.New([]mino.Address{fake.NewBadAddress()}, nil)
	_, err = format.Encode(ctx, ro)
	require.EqualError(t, err, "couldn't marshal address: fake error")

	ro = roster.New([]mino.Address{fake.NewAddress(0)}, []crypto.PublicKey{fake.NewBadPublicKey()})
	_, err = format.Encode(ctx, ro)
	require.EqualError(t, err, "couldn't serialize public key: fake error")
}

func TestRosterFormat_Decode(t *testing.T) {
	format := rosterFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})
	ctx = serde.WithFactory(ctx, roster.AddrKeyFac{}, fake.AddressFactory{})
	ctx = serde.WithFactory(ctx, roster.PubKeyFac{}, fake.PublicKeyFactory{})

	ro, err := format.Decode(ctx, []byte(`[{}]`))
	require.NoError(t, err)
	require.Equal(t, roster.FromAuthority(fake.NewAuthority(1, fake.NewSigner)), ro)

	_, err = format.Decode(fake.NewBadContext(), []byte(`[]`))
	require.EqualError(t, err, "couldn't deserialize roster: fake error")

	badCtx := serde.WithFactory(ctx, roster.PubKeyFac{}, fake.NewBadPublicKeyFactory())
	_, err = format.Decode(badCtx, []byte(`[{}]`))
	require.EqualError(t, err, "couldn't deserialize public key: fake error")

	badCtx = serde.WithFactory(ctx, roster.AddrKeyFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`[{}]`))
	require.EqualError(t, err, "invalid address factory of type '<nil>'")

	badCtx = serde.WithFactory(ctx, roster.PubKeyFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`[{}]`))
	require.EqualError(t, err, "invalid public key factory of type '<nil>'")
}
