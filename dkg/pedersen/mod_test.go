package pedersen

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc"
	"go.dedis.ch/dela/mino/minogrpc/routing"
	"go.dedis.ch/kyber/v3"
)

func TestPedersen_Scenario(t *testing.T) {
	n := 5

	addrFactory := minogrpc.AddressFactory{}

	treeFactory := routing.NewTreeRoutingFactory(3, addrFactory)

	pubKeys := make([]kyber.Point, n)
	privKeys := make([]kyber.Scalar, n)
	minos := make([]*minogrpc.Minogrpc, n)
	dkgs := make([]dkg.DKG, n)
	addrs := make([]mino.Address, n)

	for i := 0; i < n; i++ {
		privKeys[i] = suite.Scalar().Pick(suite.RandomStream())
		pubKeys[i] = suite.Point().Mul(privKeys[i], nil)

		port := uint16(2000 + i)
		minogrpc, err := minogrpc.NewMinogrpc("127.0.0.1", port, treeFactory)
		require.NoError(t, err)

		defer minogrpc.GracefulClose()

		minos[i] = minogrpc
		addrs[i] = minogrpc.GetAddress()
	}

	for i, minogrpc := range minos {
		for _, m := range minos {
			minogrpc.GetCertificateStore().Store(m.GetAddress(), m.GetCertificate())
		}

		dkg := NewPedersen(privKeys[i], minogrpc)

		dkgs[i] = dkg
	}

	message := []byte("Hello world")
	actors := make([]dkg.Actor, n)
	for i := 0; i < n; i++ {
		actor, err := dkgs[i].Listen()
		require.NoError(t, err)

		actors[i] = actor
	}

	players := &fakePlayers{players: addrs}

	// trying to call a decrpyt/encrypt before a setup
	_, _, _, err := actors[0].Encrypt(message)
	require.EqualError(t, err, "startRes is nil, did you call setup() first?")
	_, err = actors[0].Decrypt(players, nil, nil)
	require.EqualError(t, err, "startRes is nil, did you call setup() first?")

	// Do a setup on one of the actor
	// wrong lenght of pubKeys
	err = actors[0].Setup(players, pubKeys[1:], n)
	require.EqualError(t, err, "there should be as many players as pubKey: 5 := 4")

	err = actors[0].Setup(players, pubKeys, n)
	require.NoError(t, err)

	err = actors[0].Setup(players, pubKeys, n)
	require.EqualError(t, err, "startRes is not nil, only one setup call is allowed")

	// every node should be able to encrypt/decrypt
	for i := 0; i < n; i++ {
		K, C, remainder, err := actors[i].Encrypt(message)
		require.NoError(t, err)
		require.Len(t, remainder, 0)

		players = &fakePlayers{players: addrs}
		decrypted, err := actors[i].Decrypt(players, K, C)
		require.NoError(t, err)

		require.Equal(t, message, decrypted)
	}
}

// ----------------------------------------------------------------------------
// Utility functions

// fakePlayers is a fake players
//
// - implements mino.Players
type fakePlayers struct {
	players  []mino.Address
	iterator *fakeAddressIterator
}

// AddressIterator implements mino.Players
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

// Take implements mino.Players
func (p *fakePlayers) Take(filters ...mino.FilterUpdater) mino.Players {
	f := mino.ApplyFilters(filters)
	players := make([]mino.Address, len(p.players))
	for i, k := range f.Indices {
		players[i] = p.players[k]
	}
	return &fakePlayers{
		players: players,
	}
}

// fakeAddressIterator is a fake addressIterator
//
// - implements mino.addressIterator
type fakeAddressIterator struct {
	players []mino.Address
	cursor  int
}

// Seek implements mino.AddressIterator.
func (it *fakeAddressIterator) Seek(index int) {
	it.cursor = index
}

// HasNext implements mino.AddressIterator
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
