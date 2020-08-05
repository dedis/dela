package gossip

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/tap/anon"
	"go.dedis.ch/dela/core/tap/pool"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/gossip"
	"go.dedis.ch/dela/mino/minoch"
	"golang.org/x/xerrors"
)

func TestPool_Basic(t *testing.T) {
	_, pools := makeRoster(t, 10)
	defer func() {
		for _, pool := range pools {
			require.NoError(t, pool.Close())
		}
	}()

	wg := sync.WaitGroup{}
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			err := pools[0].Add(makeTx(t, uint64(i)))
			require.NoError(t, err)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			err := pools[2].Add(makeTx(t, uint64(i)+100))
			require.NoError(t, err)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			err := pools[7].Add(makeTx(t, uint64(i)+200))
			require.NoError(t, err)
		}
	}()

	wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evts := pools[9].Watch(ctx)

	err := pools[8].Add(makeTx(t, 0))
	require.NoError(t, err)

	waitEvt(t, evts, 300)
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
			"A": makeTx(t, 0),
			"B": makeTx(t, 1),
			"C": makeTx(t, 2),
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

	err := pool.Add(makeTx(t, 0))
	require.NoError(t, err)
	require.Len(t, pool.bag, 1)

	waitEvt(t, evts, 1)

	pool.actor = fakeActor{err: xerrors.New("oops")}
	err = pool.Add(makeTx(t, 0))
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

	tx := makeTx(t, 0)
	pool.bag[string(tx.GetID())] = tx

	err := pool.Remove(tx)
	require.NoError(t, err)
	require.Len(t, pool.bag, 2)

	waitEvt(t, evts, 2)
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

func makeTx(t *testing.T, nonce uint64) tap.Transaction {
	tx, err := anon.NewTransaction(nonce)
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

func waitEvt(t *testing.T, ch <-chan pool.Event, expected int) {
	select {
	case evt := <-ch:
		require.Equal(t, expected, evt.Len)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
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
