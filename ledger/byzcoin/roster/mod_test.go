package roster

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/consensus/viewchange"
	"go.dedis.ch/fabric/crypto/bls"
	"go.dedis.ch/fabric/encoding"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/mino"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&Roster{},
		&Task{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestIterator_HasNext(t *testing.T) {
	iter := &iterator{
		roster: &roster{addrs: make([]mino.Address, 3)},
	}

	require.True(t, iter.HasNext())

	iter.index = 1
	require.True(t, iter.HasNext())

	iter.index = 2
	require.True(t, iter.HasNext())

	iter.index = 3
	require.False(t, iter.HasNext())

	iter.index = 10
	require.False(t, iter.HasNext())
}

func TestIterator_GetNext(t *testing.T) {
	iter := &iterator{
		roster: &roster{addrs: make([]mino.Address, 3)},
	}

	for i := 0; i < 3; i++ {
		c := iter.GetNext()
		require.NotNil(t, c)
	}

	require.Equal(t, 3, iter.GetNext())
}

func TestAddressIterator_GetNext(t *testing.T) {
	roster := rosterFactory{}.New(fake.NewAuthority(3, fake.NewSigner)).(roster)
	iter := &addressIterator{
		iterator: &iterator{
			roster: &roster,
		},
	}

	for _, target := range roster.addrs {
		addr := iter.GetNext()
		require.Equal(t, target, addr)
	}

	require.Nil(t, iter.GetNext())
}

func TestPublicKeyIterator_Seek(t *testing.T) {
	roster := rosterFactory{}.New(fake.NewAuthority(3, fake.NewSigner)).(roster)
	iter := &publicKeyIterator{
		iterator: &iterator{
			roster: &roster,
		},
	}

	iter.Seek(2)
	require.True(t, iter.HasNext())
	iter.Seek(3)
	require.False(t, iter.HasNext())
}

func TestPublicKeyIterator_GetNext(t *testing.T) {
	roster := rosterFactory{}.New(fake.NewAuthority(3, fake.NewSigner)).(roster)
	iter := &publicKeyIterator{
		iterator: &iterator{
			roster: &roster,
		},
	}

	for _, target := range roster.pubkeys {
		pubkey := iter.GetNext()
		require.Equal(t, target, pubkey)
	}

	require.Nil(t, iter.GetNext())
}

func TestRoster_Take(t *testing.T) {
	roster := rosterFactory{}.New(fake.NewAuthority(3, fake.NewSigner))

	roster2 := roster.Take(mino.RangeFilter(1, 2))
	require.Equal(t, 1, roster2.Len())

	roster2 = roster.Take(mino.RangeFilter(1, 3))
	require.Equal(t, 2, roster2.Len())
}

func TestRoster_Apply(t *testing.T) {
	roster := rosterFactory{}.New(fake.NewAuthority(3, fake.NewSigner))

	roster2 := roster.Apply(viewchange.ChangeSet{Remove: []uint32{3, 2, 0}})
	require.Equal(t, roster.Len()-2, roster2.Len())

	roster3 := roster2.Apply(viewchange.ChangeSet{Add: []viewchange.Player{{}}})
	require.Equal(t, roster.Len()-1, roster3.Len())
}

func TestRoster_Len(t *testing.T) {
	roster := rosterFactory{}.New(fake.NewAuthority(3, fake.NewSigner))
	require.Equal(t, 3, roster.Len())
}

func TestRoster_GetPublicKey(t *testing.T) {
	authority := fake.NewAuthority(3, fake.NewSigner)
	roster := rosterFactory{}.New(authority)

	iter := authority.AddressIterator()
	i := 0
	for iter.HasNext() {
		pubkey, index := roster.GetPublicKey(iter.GetNext())
		require.Equal(t, authority.GetSigner(i).GetPublicKey(), pubkey)
		require.Equal(t, i, index)
		i++
	}

	pubkey, index := roster.GetPublicKey(fake.NewAddress(999))
	require.Equal(t, -1, index)
	require.Nil(t, pubkey)
}

func TestRoster_AddressIterator(t *testing.T) {
	authority := fake.NewAuthority(3, fake.NewSigner)
	roster := rosterFactory{}.New(authority)

	iter := roster.AddressIterator()
	for i := 0; iter.HasNext(); i++ {
		require.Equal(t, authority.GetAddress(i), iter.GetNext())
	}
}

func TestRoster_PublicKeyIterator(t *testing.T) {
	authority := fake.NewAuthority(3, bls.NewSigner)
	roster := rosterFactory{}.New(authority)

	iter := roster.PublicKeyIterator()
	for i := 0; iter.HasNext(); i++ {
		require.Equal(t, authority.GetSigner(i).GetPublicKey(), iter.GetNext())
	}
}

func TestRoster_Pack(t *testing.T) {
	roster := rosterFactory{}.New(fake.NewAuthority(3, fake.NewSigner)).(roster)

	rosterpb, err := roster.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.NotNil(t, rosterpb)

	roster.addrs[1] = fake.NewBadAddress()
	_, err = roster.Pack(encoding.NewProtoEncoder())
	require.EqualError(t, err, "couldn't marshal address: fake error")

	_, err = roster.Pack(fake.BadPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack public key: fake error")
}

func TestRosterFactory_GetAddressFactory(t *testing.T) {
	factory := rosterFactory{
		addressFactory: fake.AddressFactory{},
	}

	require.NotNil(t, factory.GetAddressFactory())
}

func TestRosterFactory_GetPublicKeyFactory(t *testing.T) {
	factory := rosterFactory{
		pubkeyFactory: fake.PublicKeyFactory{},
	}

	require.NotNil(t, factory.GetPublicKeyFactory())
}

func TestRosterFactory_FromProto(t *testing.T) {
	roster := rosterFactory{}.New(fake.NewAuthority(3, fake.NewSigner))
	rosterpb, err := roster.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)

	factory := NewRosterFactory(fake.AddressFactory{}, fake.PublicKeyFactory{}).(rosterFactory)

	decoded, err := factory.FromProto(rosterpb)
	require.NoError(t, err)
	require.Equal(t, roster.Len(), decoded.Len())

	_, err = factory.FromProto(nil)
	require.EqualError(t, err, "invalid message type '<nil>'")

	_, err = factory.FromProto(&Roster{Addresses: [][]byte{{}}})
	require.EqualError(t, err, "mismatch array length 1 != 0")

	factory.pubkeyFactory = fake.NewBadPublicKeyFactory()
	_, err = factory.FromProto(rosterpb)
	require.EqualError(t, err, "couldn't decode public key: fake error")
}
