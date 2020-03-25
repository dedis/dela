package viewchange

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

func TestConstantViewChange_Wait(t *testing.T) {
	vc := NewConstant(fakeAddress{id: "unknown"}, fakeBlockchain{len: 1, id: "unknown"})

	players, err := vc.Wait(fakeBlock{})
	require.NoError(t, err)
	require.NotNil(t, players)

	vc.bc = fakeBlockchain{err: xerrors.New("oops")}
	_, err = vc.Wait(fakeBlock{})
	require.EqualError(t, err, "couldn't read latest block: oops")

	vc.bc = fakeBlockchain{len: 0}
	_, err = vc.Wait(fakeBlock{})
	require.EqualError(t, err, "players is empty")

	vc.bc = fakeBlockchain{len: 1, id: "unknown"}
	vc.addr = fakeAddress{id: "deadbeef"}
	_, err = vc.Wait(fakeBlock{})
	require.EqualError(t, err, "mismatching leader: unknown != deadbeef")
}

func TestConstantViewChange_Verify(t *testing.T) {
	vc := NewConstant(fakeAddress{}, fakeBlockchain{})

	err := vc.Verify(fakeBlock{})
	require.NoError(t, err)

	vc.bc = fakeBlockchain{err: xerrors.New("oops")}
	err = vc.Verify(fakeBlock{})
	require.EqualError(t, err, "couldn't read latest block: oops")

	vc.bc = fakeBlockchain{id: "B"}
	err = vc.Verify(fakeBlock{id: "A"})
	require.EqualError(t, err, "mismatching leader: A != B")
}

//------------------------------------------------------------------------------
// Utility functions

type fakeAddress struct {
	mino.Address
	id string
}

func (a fakeAddress) Equal(other mino.Address) bool {
	return a.id == other.(fakeAddress).id
}

func (a fakeAddress) String() string {
	return a.id
}

type fakeIterator struct {
	mino.AddressIterator
	id string
}

func (i fakeIterator) GetNext() mino.Address {
	return fakeAddress{id: i.id}
}

type fakePlayers struct {
	mino.Players
	len int
	id  string
}

func (p fakePlayers) Len() int {
	return p.len
}

func (p fakePlayers) AddressIterator() mino.AddressIterator {
	return fakeIterator{id: p.id}
}

type fakeBlock struct {
	blockchain.Block
	len int
	id  string
}

func (b fakeBlock) GetPlayers() mino.Players {
	return fakePlayers{len: b.len, id: b.id}
}

type fakeBlockchain struct {
	blockchain.Blockchain
	err error
	len int
	id  string
}

func (bc fakeBlockchain) GetBlock() (blockchain.Block, error) {
	return fakeBlock{len: bc.len, id: bc.id}, bc.err
}
