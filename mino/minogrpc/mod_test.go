package minogrpc

import (
	"bytes"
	fmt "fmt"
	"testing"
	"testing/quick"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/mino"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&OverlayMsg{},
		&Envelope{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func Test_NewMinogrpc(t *testing.T) {
	// The happy path
	id := "127.0.0.1:3333"

	minoRPC, err := NewMinogrpc(id)
	require.NoError(t, err)

	require.Equal(t, id, minoRPC.GetAddress().String())
	require.Equal(t, "", minoRPC.namespace)

	peer := Peer{
		Address:     id,
		Certificate: minoRPC.server.cert.Leaf,
	}

	require.Equal(t, peer, minoRPC.server.neighbours[id])

	// Giving an empty address
	minoRPC, err = NewMinogrpc("")
	require.EqualError(t, err, "identifier can't be empty")
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
	newMino, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace can not be empty")

	// A namespace should match [a-zA-Z0-9]+
	ns = "/namespace"
	newMino, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace should match [a-zA-Z0-9]+, but found '/namespace'")

	ns = " test"
	newMino, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace should match [a-zA-Z0-9]+, but found ' test'")

	ns = "test$"
	newMino, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace should match [a-zA-Z0-9]+, but found 'test$'")
}

func Test_Address(t *testing.T) {
	addr := address{
		id: "test",
	}
	minoGrpc := Minogrpc{
		server: Server{
			addr: addr,
		},
	}

	require.Equal(t, addr, minoGrpc.GetAddress())
}

func Test_MakeRPC(t *testing.T) {
	minoGrpc := Minogrpc{}
	minoGrpc.namespace = "namespace"
	minoGrpc.server = Server{
		handlers: make(map[string]mino.Handler),
	}

	handler := testSameHandler{}

	rpc, err := minoGrpc.MakeRPC("name", handler)
	require.NoError(t, err)

	expectedRPC := RPC{
		handler: handler,
		srv:     minoGrpc.server,
		uri:     fmt.Sprintf("namespace/name"),
	}

	h, ok := minoGrpc.server.handlers[expectedRPC.uri]
	require.True(t, ok)
	require.Equal(t, handler, h)

	require.Equal(t, expectedRPC, rpc)

}

func Test_Address_MarshalText(t *testing.T) {
	f := func(id string) bool {
		addr := address{id: id}
		buffer, err := addr.MarshalText()
		require.NoError(t, err)

		return bytes.Equal([]byte(id), buffer)
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func Test_Address_String(t *testing.T) {
	f := func(id string) bool {
		addr := address{id: id}

		return id == addr.String()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func Test_AddressFactory_FromText(t *testing.T) {
	f := func(id string) bool {
		factory := addressFactory{}
		addr := factory.FromText([]byte(id))

		return addr.(address).id == id
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func Test_GetAddressFactory(t *testing.T) {
	m := &Minogrpc{}
	require.IsType(t, addressFactory{}, m.GetAddressFactory())
}

func Test_Players(t *testing.T) {
	players := fakePlayers{players: []address{address{"test"}}}
	it := players.AddressIterator()
	it2, ok := it.(*fakeAddressIterator)
	require.True(t, ok)

	require.Equal(t, players.players, it2.players)

	require.Equal(t, 1, players.Len())
}

func Test_AddressIterator(t *testing.T) {
	a := address{"test"}
	it := fakeAddressIterator{
		players: []address{a},
	}

	require.True(t, it.HasNext())
	addr := it.GetNext()
	require.Equal(t, a, addr)

	require.False(t, it.HasNext())
}

// -----------------
// Utility functions

// fakePlayers implements mino.Players{}
type fakePlayers struct {
	players  []address
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
	players []address
	cursor  int
}

// HasNext implements mino.AddressIterator.HasNext()
func (it *fakeAddressIterator) HasNext() bool {
	if it.cursor < len(it.players) {
		return true
	}
	return false
}

// GetNext implements mino.AddressIterator.GetNext(). It is the responsibility
// of the caller to check there is still elements to get. Otherwise it may
// crash.
func (it *fakeAddressIterator) GetNext() mino.Address {
	p := it.players[it.cursor]
	it.cursor++
	return p
}
