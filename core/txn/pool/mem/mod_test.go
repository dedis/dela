package mem

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestPool_Len(t *testing.T) {
	p := NewPool()
	require.Equal(t, 0, p.Len())

	p.gatherer.Add(fakeTx{})
	require.Equal(t, 1, p.Len())
}

func TestPool_AddFilter(t *testing.T) {
	p := NewPool()

	p.AddFilter(nil)
}

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
	require.EqualError(t, err, fake.Err("store failed"))
}

func TestPool_Remove(t *testing.T) {
	p := NewPool()

	require.NoError(t, p.gatherer.Add(fakeTx{id: []byte{1}}))

	err := p.Remove(fakeTx{id: []byte{1}})
	require.NoError(t, err)

	p.gatherer = badGatherer{}
	err = p.Remove(fakeTx{id: []byte{1}})
	require.EqualError(t, err, fake.Err("store failed"))
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

func TestPool_Close(t *testing.T) {
	p := NewPool()

	err := p.Close()
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeTx struct {
	txn.Transaction

	id []byte
}

func (tx fakeTx) GetNonce() uint64 {
	return 0
}

func (tx fakeTx) GetIdentity() access.Identity {
	return fake.PublicKey{}
}

func (tx fakeTx) GetID() []byte {
	return tx.id
}

type badGatherer struct {
	pool.Gatherer
}

func (g badGatherer) Add(tx txn.Transaction) error {
	return fake.GetError()
}

func (g badGatherer) Remove(tx txn.Transaction) error {
	return fake.GetError()
}
