package pool

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestSimpleGatherer_Len(t *testing.T) {
	gatherer := NewSimpleGatherer().(*simpleGatherer)
	require.Equal(t, 0, gatherer.Len())

	gatherer.txs["Alice"] = transactions{fakeTx{}}
	require.Equal(t, 1, gatherer.Len())

	gatherer.txs["Bob"] = transactions{fakeTx{}, fakeTx{}}
	require.Equal(t, 3, gatherer.Len())
}

func TestSimpleGatherer_Add(t *testing.T) {
	gatherer := NewSimpleGatherer().(*simpleGatherer)
	gatherer.AddFilter(nil)
	gatherer.AddFilter(fakeFilter{})

	for i := 0; i < DefaultIdentitySize; i++ {
		err := gatherer.Add(newTx(uint64(i), "Alice"))
		require.NoError(t, err)
	}

	require.Equal(t, DefaultIdentitySize, gatherer.Len())

	err := gatherer.Add(newTx(DefaultIdentitySize-1, "Alice"))
	require.NoError(t, err)

	err = gatherer.Add(newTx(5, "Bob"))
	require.NoError(t, err)

	err = gatherer.Add(newTx(2, "Bob"))
	require.NoError(t, err)

	require.Len(t, gatherer.txs["Bob"], 2)
	require.Equal(t, uint64(2), gatherer.txs["Bob"][0].GetNonce())

	err = gatherer.Add(newTx(DefaultIdentitySize+1, "Alice"))
	require.EqualError(t, err, fake.Err("invalid transaction"))

	err = gatherer.Add(fakeTx{identity: fake.NewBadPublicKey()})
	require.EqualError(t, err, fake.Err("identity key failed"))
}

func TestSimpleGatherer_Remove(t *testing.T) {
	gatherer := NewSimpleGatherer().(*simpleGatherer)
	gatherer.txs["Alice"] = transactions{newTx(0, "Alice"), newTx(1, "Alice")}

	err := gatherer.Remove(newTx(0, "Alice"))
	require.NoError(t, err)
	require.Len(t, gatherer.txs["Alice"], 1)

	err = gatherer.Remove(newTx(0, "Alice"))
	require.NoError(t, err)
	require.Len(t, gatherer.txs["Alice"], 1)

	err = gatherer.Remove(newTx(1, "Alice"))
	require.NoError(t, err)
	require.Len(t, gatherer.txs["Alice"], 0)

	err = gatherer.Remove(fakeTx{identity: fake.NewBadPublicKey()})
	require.EqualError(t, err, fake.Err("identity key failed"))
}

func TestSimpleGatherer_Wait(t *testing.T) {
	gatherer := NewSimpleGatherer().(*simpleGatherer)

	ctx := context.Background()

	cb := func() {
		gatherer.Lock()
		require.Len(t, gatherer.queue, 1)
		gatherer.Unlock()

		require.NoError(t, gatherer.Add(newTx(0xa, "Alice")))
	}

	txs := gatherer.Wait(ctx, Config{Min: 1, Callback: cb})
	require.Len(t, txs, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	txs = gatherer.Wait(ctx, Config{Min: 1})
	require.Len(t, txs, 1)

	txs = gatherer.Wait(ctx, Config{Min: 2})
	require.Nil(t, txs)
}

func TestSimpleGatherer_Close(t *testing.T) {
	gatherer := NewSimpleGatherer().(*simpleGatherer)

	require.NoError(t, gatherer.Add(newTx(0, "Alice")))
	require.NoError(t, gatherer.Add(newTx(1, "Alice")))
	require.NoError(t, gatherer.Remove(newTx(0, "Alice")))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		txs := gatherer.Wait(ctx, Config{Min: 100, Callback: wg.Done})
		require.Empty(t, txs)
	}()

	wg.Wait()
	wg.Add(1)

	gatherer.Close()

	require.Empty(t, gatherer.queue)
	require.Empty(t, gatherer.txs)

	wg.Wait()
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeTx struct {
	txn.Transaction

	id       uint64
	identity access.Identity
}

func newTx(nonce uint64, identity string) fakeTx {
	return fakeTx{
		id:       nonce,
		identity: fakeIdentity{text: identity},
	}
}

func (tx fakeTx) GetID() []byte {
	return []byte{byte(tx.id)}
}

func (tx fakeTx) GetNonce() uint64 {
	return tx.id
}

func (tx fakeTx) GetIdentity() access.Identity {
	return tx.identity
}

type fakeIdentity struct {
	access.Identity
	text string
}

func (id fakeIdentity) MarshalText() ([]byte, error) {
	return []byte(id.text), nil
}

type fakeFilter struct{}

func (fakeFilter) Accept(tx txn.Transaction, leeway validation.Leeway) error {
	if tx.GetNonce() >= uint64(leeway.MaxSequenceDifference) {
		return fake.GetError()
	}

	return nil
}
