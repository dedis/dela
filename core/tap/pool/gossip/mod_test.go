package gossip

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/tap/anon"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/gossip"
	"go.dedis.ch/dela/mino/minoch"
	"golang.org/x/xerrors"
)

func TestPool_Basic(t *testing.T) {
	_, pools := makeRoster(t, 3)
	defer func() {
		for _, pool := range pools {
			require.NoError(t, pool.Close())
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evts := pools[2].Watch(ctx)

	err := pools[0].Add(makeTx(t))
	require.NoError(t, err)

	evt := <-evts
	require.Equal(t, 1, evt.Len)
}

func TestPool_New(t *testing.T) {
	pool, err := NewPool(fakeGossiper{})
	require.NoError(t, err)
	require.NotNil(t, pool)

	err = pool.Close()
	require.NoError(t, err)

	_, err = NewPool(fakeGossiper{err: xerrors.New("oops")})
	require.EqualError(t, err, "failed to listen: oops")
}

func TestPool_Len(t *testing.T) {
	pool := &Pool{}
	require.Equal(t, 0, pool.Len())

	pool.bag = map[string]tap.Transaction{"": nil, "A": nil}
	require.Equal(t, 2, pool.Len())
}

func TestPool_GetAll(t *testing.T) {
	pool := &Pool{
		bag: map[string]tap.Transaction{
			"A": makeTx(t),
			"B": makeTx(t),
			"C": makeTx(t),
		},
	}

	txs := pool.GetAll()
	require.Len(t, txs, 3)
}

func TestPool_Add(t *testing.T) {
	pool := &Pool{
		actor:   fakeActor{},
		bag:     make(map[string]tap.Transaction),
		watcher: blockchain.NewWatcher(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evts := pool.Watch(ctx)

	err := pool.Add(makeTx(t))
	require.NoError(t, err)
	require.Len(t, pool.bag, 1)

	evt := <-evts
	require.Equal(t, 1, evt.Len)

	pool.actor = fakeActor{err: xerrors.New("oops")}
	err = pool.Add(makeTx(t))
	require.EqualError(t, err, "failed to gossip tx: oops")
}

func TestPool_Remove(t *testing.T) {
	pool := &Pool{
		actor:   fakeActor{},
		bag:     map[string]tap.Transaction{"A": nil, "B": nil},
		watcher: blockchain.NewWatcher(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evts := pool.Watch(ctx)

	tx := makeTx(t)
	pool.bag[string(tx.GetID())] = tx

	err := pool.Remove(tx)
	require.NoError(t, err)
	require.Len(t, pool.bag, 2)

	evt := <-evts
	require.Equal(t, 2, evt.Len)
}

func TestPool_Close(t *testing.T) {
	pool := &Pool{
		closing: make(chan struct{}),
		actor:   fakeActor{},
	}

	err := pool.Close()
	require.NoError(t, err)

	pool.closing = make(chan struct{})
	pool.actor = fakeActor{err: xerrors.New("oops")}
	err = pool.Close()
	require.EqualError(t, err, "failed to close gossiper: oops")
}

// Utility functions -----------------------------------------------------------

func makeTx(t *testing.T) tap.Transaction {
	tx, err := anon.NewTransaction(0)
	require.NoError(t, err)
	return tx
}

func makeRoster(t *testing.T, n int) (mino.Players, []*Pool) {
	manager := minoch.NewManager()

	pools := make([]*Pool, n)
	addrs := make([]mino.Address, n)

	for i := 0; i < n; i++ {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)

		addrs[i] = m.GetAddress()

		g := gossip.NewFlat(m, anon.NewTransactionFactory())

		pool, err := NewPool(g)
		require.NoError(t, err)

		pools[i] = pool
	}

	players := mino.NewAddresses(addrs...)
	for _, pool := range pools {
		pool.SetPlayers(players)
	}

	return players, pools
}

type fakeActor struct {
	gossip.Actor
	call *fake.Call
	err  error
}

func (a fakeActor) Add(r gossip.Rumor) error {
	a.call.Add(r)
	return a.err
}

func (a fakeActor) Close() error {
	return a.err
}

type fakeGossiper struct {
	err error
}

func (g fakeGossiper) Listen() (gossip.Actor, error) {
	return fakeActor{}, g.err
}

func (g fakeGossiper) Rumors() <-chan gossip.Rumor {
	return nil
}
