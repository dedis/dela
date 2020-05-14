package pedersen

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/dkg"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minogrpc"
	"go.dedis.ch/fabric/mino/minogrpc/routing"
	"go.dedis.ch/kyber/v3"
)

func TestPedersen_Scenario(t *testing.T) {
	n := 10

	addrFactory := minogrpc.AddressFactory{}

	rootAddr := addrFactory.FromText([]byte("127.0.0.1:2000"))

	treeFactory := routing.NewTreeRoutingFactory(3, rootAddr, addrFactory, minogrpc.OrchestratorID)

	pubKeys := make([]kyber.Point, n)
	privKeys := make([]kyber.Scalar, n)
	minos := make([]*minogrpc.Minogrpc, n)
	dkgs := make([]dkg.DKG, n)
	urls := make([]*url.URL, n)
	addrs := make([]mino.Address, n)

	for i := 0; i < n; i++ {
		url, err := url.Parse(fmt.Sprintf("//127.0.0.1:2%03d", i))
		require.NoError(t, err)

		privKeys[i] = suite.Scalar().Pick(suite.RandomStream())
		pubKeys[i] = suite.Point().Mul(privKeys[i], nil)

		minogrpc, err := minogrpc.NewMinogrpc(url, treeFactory)
		require.NoError(t, err)

		minos[i] = minogrpc
		urls[i] = url
		addrs[i] = minogrpc.GetAddress()
	}

	for i, minogrpc := range minos {
		for j, m := range minos {
			err := minogrpc.AddCertificate(urls[j], m.GetPublicCertificate())
			require.NoError(t, err)
		}

		dkg, err := NewPedersen(privKeys[i], minogrpc, suite)
		require.NoError(t, err)

		dkgs[i] = dkg
	}

	message := []byte("Hello world")
	// we try with the first 3 DKG Actors
	for i := 0; i < 3; i++ {
		players := &fakePlayers{
			players: addrs,
		}

		dkg := dkgs[i]

		actor, err := dkg.Listen(players, pubKeys, uint32(n))
		require.NoError(t, err)

		K, C, remainder, err := actor.Encrypt(message)
		require.NoError(t, err)
		require.Len(t, remainder, 0)

		decrypted, err := actor.Decrypt(K, C)
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
