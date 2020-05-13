package minogrpc

import (
	"bytes"
	"net/url"
	"testing"
	"testing/quick"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/mino"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&Envelope{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func Test_NewMinogrpc(t *testing.T) {
	// The happy path
	urlStr := "//127.0.0.1:3333"
	url, err := url.Parse(urlStr)
	require.NoError(t, err)

	minoRPC, err := NewMinogrpc(url, nil)
	require.NoError(t, err)

	require.Equal(t, "127.0.0.1:3333", minoRPC.GetAddress().String())
	require.Equal(t, "", minoRPC.namespace)

	cert, found := minoRPC.server.nodesCerts.Load("127.0.0.1:3333")
	require.True(t, found)

	require.Equal(t, minoRPC.server.cert.Leaf, cert)

	// Giving a wrong address, should be "//example:3333"
	urlStr = "example:3333"
	url, err = url.Parse(urlStr)
	require.NoError(t, err)

	_, err = NewMinogrpc(url, nil)
	require.EqualError(t, err, "host URL is invalid: empty host for 'example:3333'. Hint: the url must be created using an absolute path, like //127.0.0.1:3333")
}

func Test_MakeNamespace(t *testing.T) {
	minoGrpc := Minogrpc{}
	ns := "Test"
	newMino, err := minoGrpc.MakeNamespace(ns)
	require.NoError(t, err)

	newMinoGrpc, ok := newMino.(Minogrpc)
	require.True(t, ok)

	require.Equal(t, ns, newMinoGrpc.namespace)

	// A namespace can not be empty
	ns = ""
	_, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace can not be empty")

	// A namespace should match [a-zA-Z0-9]+
	ns = "/namespace"
	_, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace should match [a-zA-Z0-9]+, but found '/namespace'")

	ns = " test"
	_, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace should match [a-zA-Z0-9]+, but found ' test'")

	ns = "test$"
	_, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace should match [a-zA-Z0-9]+, but found 'test$'")
}

func Test_Address(t *testing.T) {
	addr := address{
		id: "test",
	}
	minoGrpc := Minogrpc{
		server: &Server{
			addr: addr,
		},
	}

	require.Equal(t, addr, minoGrpc.GetAddress())
}

func Test_MakeRPC(t *testing.T) {
	minoGrpc := Minogrpc{}
	minoGrpc.namespace = "namespace"
	minoGrpc.server = &Server{
		handlers: make(map[string]mino.Handler),
	}

	handler := testSameHandler{}

	rpc, err := minoGrpc.MakeRPC("name", handler)
	require.NoError(t, err)

	expectedRPC := &RPC{
		encoder: encoding.NewProtoEncoder(),
		handler: handler,
		srv:     minoGrpc.server,
		uri:     "namespace/name",
	}

	h, ok := minoGrpc.server.handlers[expectedRPC.uri]
	require.True(t, ok)
	require.Equal(t, handler, h)

	require.Equal(t, expectedRPC, rpc)

}

func TestAddress_Equal(t *testing.T) {
	addr := address{id: "A"}
	require.True(t, addr.Equal(addr))
	require.False(t, addr.Equal(address{}))
}

func TestAddress_MarshalText(t *testing.T) {
	f := func(id string) bool {
		addr := address{id: id}
		buffer, err := addr.MarshalText()
		require.NoError(t, err)

		return bytes.Equal([]byte(id), buffer)
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestAddress_String(t *testing.T) {
	f := func(id string) bool {
		addr := address{id: id}

		return id == addr.String()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestAddressFactory_FromText(t *testing.T) {
	f := func(id string) bool {
		factory := AddressFactory{}
		addr := factory.FromText([]byte(id))

		return addr.(address).id == id
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestMinogrpc_GetAddressFactory(t *testing.T) {
	m := &Minogrpc{}
	require.IsType(t, AddressFactory{}, m.GetAddressFactory())
}

func TestPlayers_AddressIterator(t *testing.T) {
	players := fakePlayers{players: []mino.Address{address{"test"}}}
	it := players.AddressIterator()
	it2, ok := it.(*fakeAddressIterator)
	require.True(t, ok)

	require.Equal(t, players.players, it2.players)

	require.Equal(t, 1, players.Len())
}

func TestAddressIterator(t *testing.T) {
	a := &address{"test"}
	it := fakeAddressIterator{
		players: []mino.Address{a},
	}

	require.True(t, it.HasNext())
	addr := it.GetNext()
	require.Equal(t, a, addr)

	require.False(t, it.HasNext())
}

// -----------------------------------------------------------------------------
// Utility functions

// fakePlayers implements mino.Players{}
type fakePlayers struct {
	mino.Players
	players  []mino.Address
	iterator *fakeAddressIterator
}

// AddressIterator implements mino.Players.AddressIterator()
func (p *fakePlayers) AddressIterator() mino.AddressIterator {
	if p.iterator == nil {
		p.iterator = &fakeAddressIterator{players: p.players}
	}
	return p.iterator
}

// Len() implements mino.Players.Len()
func (p *fakePlayers) Len() int {
	return len(p.players)
}

// fakeAddressIterator implements mino.addressIterator{}
type fakeAddressIterator struct {
	players []mino.Address
	cursor  int
}

// HasNext implements mino.AddressIterator.HasNext()
func (it *fakeAddressIterator) HasNext() bool {
	return it.cursor < len(it.players)
}

// GetNext implements mino.AddressIterator.GetNext(). It is the responsibility
// of the caller to check there is still elements to get. Otherwise it may
// crash.
func (it *fakeAddressIterator) GetNext() mino.Address {
	p := it.players[it.cursor]
	it.cursor++
	return p
}
