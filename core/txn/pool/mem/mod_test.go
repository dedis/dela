package mem

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	"golang.org/x/xerrors"
)

func TestPool_Add(t *testing.T) {
	p := NewPool()

	err := p.Add(fakeTx{id: []byte{1}})
	require.NoError(t, err)

	err = p.Add(fakeTx{id: []byte{2}})
	require.NoError(t, err)

	// A transaction that exists in the active queue can simply be overwritten
	// thus no error is expected.
	err = p.Add(fakeTx{id: []byte{1}})
	require.NoError(t, err)

	p.gatherer = badGatherer{}
	err = p.Add(fakeTx{})
	require.EqualError(t, err, "store failed: oops")
}

func TestPool_Remove(t *testing.T) {
	p := NewPool()

	require.NoError(t, p.gatherer.Add(fakeTx{id: []byte{1}}))

	err := p.Remove(fakeTx{id: []byte{1}})
	require.NoError(t, err)

	p.gatherer = badGatherer{}
	err = p.Remove(fakeTx{id: []byte{1}})
	require.EqualError(t, err, "store failed: oops")
}

func TestPool_SetPlayers(t *testing.T) {
	pool := NewPool()

	require.NoError(t, pool.SetPlayers(nil))
}

func TestPool_Gather(t *testing.T) {
	p := NewPool()

	ctx := context.Background()

	cb := func() {
		require.NoError(t, p.Add(fakeTx{}))
	}

	txs := p.Gather(ctx, pool.Config{Min: 1, Callback: cb})
	require.Len(t, txs, 1)

	txs = p.Gather(ctx, pool.Config{Min: 1})
	require.Len(t, txs, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	txs = p.Gather(ctx, pool.Config{Min: 2})
	require.Len(t, txs, 0)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeTx struct {
	txn.Transaction

	id []byte
}

func (tx fakeTx) GetID() []byte {
	return tx.id
}

type badGatherer struct {
	pool.Gatherer
}

func (g badGatherer) Add(tx txn.Transaction) error {
	return xerrors.New("oops")
}

func (g badGatherer) Remove(tx txn.Transaction) error {
	return xerrors.New("oops")
}
