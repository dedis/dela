package viewchange

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

func TestConstantViewChange_Wait(t *testing.T) {
	vc := NewConstant(fake.NewAddress(0), fakeBlockchain{len: 1})

	players, err := vc.Wait(fakeBlock{})
	require.NoError(t, err)
	require.NotNil(t, players)

	vc.bc = fakeBlockchain{err: xerrors.New("oops")}
	_, err = vc.Wait(fakeBlock{})
	require.EqualError(t, err, "couldn't read latest block: oops")

	vc.bc = fakeBlockchain{len: 0}
	_, err = vc.Wait(fakeBlock{})
	require.EqualError(t, err, "players is empty")

	vc.bc = fakeBlockchain{len: 1}
	vc.addr = fake.NewAddress(5)
	_, err = vc.Wait(fakeBlock{})
	require.EqualError(t, err, "mismatching leader: fake.Address[0] != fake.Address[5]")
}

func TestConstantViewChange_Verify(t *testing.T) {
	vc := NewConstant(fake.Address{}, fakeBlockchain{len: 1})

	err := vc.Verify(fakeBlock{len: 1})
	require.NoError(t, err)

	vc.bc = fakeBlockchain{err: xerrors.New("oops")}
	err = vc.Verify(fakeBlock{len: 1})
	require.EqualError(t, err, "couldn't read latest block: oops")

	vc.bc = fakeBlockchain{len: 1}
	err = vc.Verify(fakeBlock{len: 1, base: 1})
	require.EqualError(t, err, "mismatching leader: fake.Address[1] != fake.Address[0]")
}

//------------------------------------------------------------------------------
// Utility functions

type fakeBlock struct {
	blockchain.Block
	len  int
	base int
}

func (b fakeBlock) GetPlayers() mino.Players {
	return fake.NewAuthorityWithBase(b.base, b.len, fake.NewSigner)
}

type fakeBlockchain struct {
	blockchain.Blockchain
	err error
	len int
}

func (bc fakeBlockchain) GetBlock() (blockchain.Block, error) {
	return fakeBlock{len: bc.len}, bc.err
}
