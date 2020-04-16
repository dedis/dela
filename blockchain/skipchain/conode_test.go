package skipchain

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/mino"
)

func TestConode_GetAddress(t *testing.T) {
	addr := fake.Address{}
	conode := Conode{addr: addr}

	require.Equal(t, addr, conode.GetAddress())
}

func TestConode_GetPublicKey(t *testing.T) {
	pk := fake.PublicKey{}
	conode := Conode{publicKey: pk}

	require.Equal(t, pk, conode.GetPublicKey())
}

func TestConode_Pack(t *testing.T) {
	conode := Conode{
		addr:      fake.Address{},
		publicKey: fake.PublicKey{},
	}

	pb, err := conode.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*ConodeProto)(nil), pb)

	_, err = conode.Pack(fake.BadPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack public key: fake error")

	conode.addr = fake.NewBadAddress()
	_, err = conode.Pack(encoding.NewProtoEncoder())
	require.EqualError(t, err, "couldn't marshal address: fake error")
}

func TestIterator_HasNext(t *testing.T) {
	iter := &iterator{
		conodes: []Conode{{}, {}, {}},
		index:   -1,
	}

	require.True(t, iter.HasNext())

	iter.index = 0
	require.True(t, iter.HasNext())

	iter.index = 1
	require.True(t, iter.HasNext())

	iter.index = 2
	require.False(t, iter.HasNext())

	iter.index = 10
	require.False(t, iter.HasNext())
}

func TestIterator_GetNext(t *testing.T) {
	iter := &iterator{
		conodes: []Conode{{}, {}, {}},
		index:   -1,
	}

	for i := 0; i < 3; i++ {
		c := iter.GetNext()
		require.NotNil(t, c)
	}

	require.Nil(t, iter.GetNext())
}

func TestAddressIterator_GetNext(t *testing.T) {
	conodes := []Conode{randomConode(), randomConode(), randomConode()}
	iter := &addressIterator{
		iterator: &iterator{
			conodes: conodes,
			index:   -1,
		},
	}

	for _, conode := range conodes {
		addr := iter.GetNext()
		require.Equal(t, conode.GetAddress(), addr)
	}

	require.Nil(t, iter.GetNext())
}

func TestPublicKeyIterator_GetNext(t *testing.T) {
	conodes := []Conode{randomConode(), randomConode(), randomConode()}
	iter := &publicKeyIterator{
		iterator: &iterator{
			conodes: conodes,
			index:   -1,
		},
	}

	for _, conode := range conodes {
		pubkey := iter.GetNext()
		require.Equal(t, conode.GetPublicKey(), pubkey)
	}

	require.Nil(t, iter.GetNext())
}

func TestConodes_Take(t *testing.T) {
	conodes := Conodes{randomConode(), randomConode(), randomConode()}

	for i := range conodes {
		subset := conodes.Take(mino.IndexFilter(i))
		require.Equal(t, conodes[i], subset.(Conodes)[0])
	}
}

func TestConodes_Len(t *testing.T) {
	conodes := Conodes{randomConode(), randomConode()}
	require.Equal(t, 2, conodes.Len())

	conodes = Conodes{}
	require.Equal(t, 0, conodes.Len())
}

func TestConodes_GetPublicKey(t *testing.T) {
	conodes := Conodes{randomConode(), randomConode(), randomConode()}

	pubkey, index := conodes.GetPublicKey(conodes[1].GetAddress())
	require.Equal(t, 1, index)
	require.Equal(t, conodes[1].GetPublicKey(), pubkey)

	pubkey, index = conodes.GetPublicKey(fake.Address{})
	require.Equal(t, -1, index)
	require.Nil(t, pubkey)
}

func TestConodes_AddressIterator(t *testing.T) {
	conodes := Conodes{randomConode(), randomConode()}
	iter := conodes.AddressIterator()

	for range conodes {
		require.True(t, iter.HasNext())
		require.NotNil(t, iter.GetNext())
	}
	require.False(t, iter.HasNext())
}

func TestConodes_PublicKeyIterator(t *testing.T) {
	conodes := Conodes{randomConode(), randomConode()}
	iter := conodes.PublicKeyIterator()

	for range conodes {
		require.True(t, iter.HasNext())
		require.NotNil(t, iter.GetNext())
	}
	require.False(t, iter.HasNext())
}

func TestConodes_Pack(t *testing.T) {
	conodes := Conodes{randomConode(), randomConode()}

	pb, err := conodes.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*Roster)(nil), pb)
	require.Len(t, pb.(*Roster).GetConodes(), 2)

	_, err = conodes.Pack(fake.BadPackEncoder{})
	require.EqualError(t, err, "couldn't pack conode: fake error")
}

func TestConodes_WriteTo(t *testing.T) {
	conodes := Conodes{randomConode(), randomConode()}

	buffer := new(bytes.Buffer)
	sum, err := conodes.WriteTo(buffer)
	require.NoError(t, err)
	require.Equal(t, sum, int64(buffer.Len()))

	_, err = conodes.WriteTo(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write public key: fake error")

	_, err = conodes.WriteTo(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err, "couldn't write address: fake error")

	conodes[1].publicKey = fake.NewBadPublicKey()
	_, err = conodes.WriteTo(buffer)
	require.EqualError(t, err, "couldn't marshal public key: fake error")
}
