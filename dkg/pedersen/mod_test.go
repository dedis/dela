package pedersen

import (
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/dkg"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minogrpc"
	"go.dedis.ch/fabric/mino/minogrpc/routing"
	"go.dedis.ch/kyber/v3"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&Start{},
		&Deal{},
		&Deal_EncryptedDeal{},
		&Response{},
		&Response_Data{},
		&StartDone{},
		&DecryptRequest{},
		&DecryptReply{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

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

		minos[i] = minogrpc
		addrs[i] = minogrpc.GetAddress()
	}

	for i, minogrpc := range minos {
		for _, m := range minos {
			err := minogrpc.AddCertificate(m.GetAddress(), m.GetCertificate())
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
