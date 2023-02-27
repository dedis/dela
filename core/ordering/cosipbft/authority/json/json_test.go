package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

func TestChangeSetFormat_Encode(t *testing.T) {
	cset := authority.NewChangeSet()
	cset.Remove(42)
	cset.Add(fake.NewAddress(2), fake.PublicKey{})

	format := changeSetFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, cset)
	require.NoError(t, err)
	expected := `{"Remove":[42],"Addresses":["AgAAAA=="],"PublicKeys":[{}]}`
	require.Equal(t, expected, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")

	_, err = format.Encode(fake.NewBadContext(), cset)
	require.EqualError(t, err, fake.Err("couldn't marshal"))

	cset = authority.NewChangeSet()
	cset.Add(fake.NewAddress(0), fake.NewBadPublicKey())
	_, err = format.Encode(ctx, cset)
	require.EqualError(t, err, fake.Err("couldn't serialize public key"))

	cset = authority.NewChangeSet()
	cset.Add(fake.NewBadAddress(), fake.PublicKey{})
	_, err = format.Encode(ctx, cset)
	require.EqualError(t, err, fake.Err("couldn't serialize address"))
}

func TestChangeSetFormat_Decode(t *testing.T) {
	format := changeSetFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})
	ctx = serde.WithFactory(ctx, authority.AddrKeyFac{}, fake.AddressFactory{})
	ctx = serde.WithFactory(ctx, authority.PubKeyFac{}, fake.PublicKeyFactory{})

	cset := authority.NewChangeSet()
	cset.Add(fake.NewAddress(0), fake.PublicKey{})

	msg, err := format.Decode(ctx, []byte(`{"Addresses":[[]],"PublicKeys":[{}]}`))
	require.NoError(t, err)
	require.Equal(t, cset, msg)

	cset = authority.NewChangeSet()
	cset.Remove(1)
	cset.Remove(2)
	cset.Remove(3)

	msg, err = format.Decode(ctx, []byte(`{"Remove":[1,2,3]}`))
	require.NoError(t, err)
	require.Equal(t, cset, msg)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("couldn't deserialize change set"))

	badCtx := serde.WithFactory(ctx, authority.PubKeyFac{}, fake.NewBadPublicKeyFactory())
	_, err = format.Decode(badCtx, []byte(`{"Addresses":[[]],"PublicKeys":[{}]}`))
	require.EqualError(t, err, fake.Err("couldn't deserialize public key"))

	badCtx = serde.WithFactory(ctx, authority.AddrKeyFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Addresses":[[]],"PublicKeys":[{}]}`))
	require.EqualError(t, err, "invalid address factory of type '<nil>'")

	badCtx = serde.WithFactory(ctx, authority.PubKeyFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Addresses":[[]],"PublicKeys":[{}]}`))
	require.EqualError(t, err, "invalid public key factory of type '<nil>'")
}

func TestRosterFormat_Encode(t *testing.T) {
	ro := authority.FromAuthority(fake.NewAuthority(1, fake.NewSigner))

	format := rosterFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, ro)
	require.NoError(t, err)
	require.Equal(t, `[{"Address":"AAAAAA==","PublicKey":{}}]`, string(data))

	_, err = format.Encode(fake.NewContext(), fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")

	_, err = format.Encode(fake.NewBadContext(), ro)
	require.EqualError(t, err, fake.Err("couldn't marshal"))

	ro = authority.New([]mino.Address{fake.NewBadAddress()}, nil)
	_, err = format.Encode(ctx, ro)
	require.EqualError(t, err, fake.Err("couldn't marshal address"))

	ro = authority.New([]mino.Address{fake.NewAddress(0)}, []crypto.PublicKey{fake.NewBadPublicKey()})
	_, err = format.Encode(ctx, ro)
	require.EqualError(t, err, fake.Err("couldn't serialize public key"))
}

func TestRosterFormat_Decode(t *testing.T) {
	format := rosterFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})
	ctx = serde.WithFactory(ctx, authority.AddrKeyFac{}, fake.AddressFactory{})
	ctx = serde.WithFactory(ctx, authority.PubKeyFac{}, fake.PublicKeyFactory{})

	ro, err := format.Decode(ctx, []byte(`[{}]`))
	require.NoError(t, err)
	require.Equal(t, authority.FromAuthority(fake.NewAuthority(1, fake.NewSigner)), ro)

	_, err = format.Decode(fake.NewBadContext(), []byte(`[]`))
	require.EqualError(t, err, fake.Err("couldn't deserialize roster"))

	badCtx := serde.WithFactory(ctx, authority.PubKeyFac{}, fake.NewBadPublicKeyFactory())
	_, err = format.Decode(badCtx, []byte(`[{}]`))
	require.EqualError(t, err, fake.Err("couldn't deserialize public key"))

	badCtx = serde.WithFactory(ctx, authority.AddrKeyFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`[{}]`))
	require.EqualError(t, err, "invalid address factory of type '<nil>'")

	badCtx = serde.WithFactory(ctx, authority.PubKeyFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`[{}]`))
	require.EqualError(t, err, "invalid public key factory of type '<nil>'")
}
