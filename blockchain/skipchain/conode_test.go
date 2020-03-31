package skipchain

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

func TestConode_GetAddress(t *testing.T) {
	addr := fakeAddress{}
	conode := Conode{addr: addr}

	require.Equal(t, addr, conode.GetAddress())
}

func TestConode_GetPublicKey(t *testing.T) {
	pk := fakePublicKey{}
	conode := Conode{publicKey: pk}

	require.Equal(t, pk, conode.GetPublicKey())
}

func TestConode_Pack(t *testing.T) {
	conode := Conode{
		addr:      fakeAddress{},
		publicKey: fakePublicKey{},
	}

	pb, err := conode.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*ConodeProto)(nil), pb)

	_, err = conode.Pack(badPackAnyEncoder{})
	require.EqualError(t, err, "encoder: oops")

	conode.addr = fakeAddress{err: xerrors.New("oops")}
	_, err = conode.Pack(encoding.NewProtoEncoder())
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewEncodingError("address", nil)))
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

	_, err = conodes.Pack(badPackEncoder{})
	require.EqualError(t, err, "encoder: oops")
}

func TestConodes_WriteTo(t *testing.T) {
	conodes := Conodes{randomConode(), randomConode()}

	buffer := new(bytes.Buffer)
	sum, err := conodes.WriteTo(buffer)
	require.NoError(t, err)
	require.Equal(t, sum, int64(buffer.Len()))

	_, err = conodes.WriteTo(&badWriter{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't write public key: oops")

	_, err = conodes.WriteTo(&badWriter{delay: 1, err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't write address: oops")

	conodes[1].publicKey = fakePublicKey{err: xerrors.New("oops")}
	_, err = conodes.WriteTo(buffer)
	require.EqualError(t, err, "couldn't marshal public key: oops")
}

//------------------
// Utility functions

type badWriter struct {
	io.Writer
	err   error
	delay int
}

func (w *badWriter) Write([]byte) (int, error) {
	if w.delay == 0 {
		return 0, w.err
	}
	w.delay--
	return 0, nil
}
