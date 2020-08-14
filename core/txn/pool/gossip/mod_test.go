package gossip

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/anon"
	"go.dedis.ch/dela/core/txn/pool"
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

	go func() {
		for i := 0; i < 100; i++ {
			err := pools[0].Add(makeTx(t, uint64(i)))
			require.NoError(t, err)
		}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			err := pools[2].Add(makeTx(t, uint64(i)+100))
			require.NoError(t, err)
		}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			err := pools[7].Add(makeTx(t, uint64(i)+200))
			require.NoError(t, err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	txs := pools[0].Gather(ctx, pool.Config{Min: 300})
	require.Len(t, txs, 300)
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

func TestPool_Add(t *testing.T) {
	p := &Pool{
		actor:    fakeActor{},
		gatherer: pool.NewSimpleGatherer(),
	}

	err := p.Add(makeTx(t, 0))
	require.NoError(t, err)

	p.gatherer = badGatherer{}
	err = p.Add(makeTx(t, 0))
	require.EqualError(t, err, "store failed: oops")

	p.gatherer = pool.NewSimpleGatherer()
	p.actor = fakeActor{err: xerrors.New("oops")}
	err = p.Add(makeTx(t, 0))
	require.EqualError(t, err, "failed to gossip tx: oops")
}

func TestPool_Remove(t *testing.T) {
	p := &Pool{
		actor:    fakeActor{},
		gatherer: pool.NewSimpleGatherer(),
	}

	tx := makeTx(t, 0)

	require.NoError(t, p.gatherer.Add(tx))

	err := p.Remove(tx)
	require.NoError(t, err)

	p.gatherer = badGatherer{}
	err = p.Remove(tx)
	require.EqualError(t, err, "store failed: oops")
}

func TestPool_Gather(t *testing.T) {
	p := &Pool{
		actor:    fakeActor{},
		gatherer: pool.NewSimpleGatherer(),
	}

	ctx := context.Background()

	cb := func() {
		require.NoError(t, p.Add(makeTx(t, 0)))
		require.NoError(t, p.Add(makeTx(t, 1)))
	}

	txs := p.Gather(ctx, pool.Config{Min: 2, Callback: cb})
	require.Len(t, txs, 2)

	txs = p.Gather(ctx, pool.Config{Min: 2})
	require.Len(t, txs, 2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	txs = p.Gather(ctx, pool.Config{Min: 3})
	require.Len(t, txs, 0)
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

func TestPool_ListenRumors(t *testing.T) {
	buffer := new(bytes.Buffer)

	p := &Pool{
		logger:   zerolog.New(buffer),
		closing:  make(chan struct{}),
		gatherer: pool.NewSimpleGatherer(),
	}

	ch := make(chan gossip.Rumor)
	go func() {
		ch <- makeTx(t, 0)
		close(p.closing)
	}()

	p.listenRumors(ch)
	require.Empty(t, buffer.String())

	p.gatherer = badGatherer{}
	p.closing = make(chan struct{})

	ch = make(chan gossip.Rumor, 1)
	go func() {
		ch <- makeTx(t, 0)
		close(p.closing)
	}()

	p.listenRumors(ch)
	require.NotEmpty(t, buffer.String())
}

// Utility functions -----------------------------------------------------------

func makeTx(t *testing.T, nonce uint64) txn.Transaction {
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

type badGatherer struct {
	pool.Gatherer
}

func (g badGatherer) Add(tx txn.Transaction) error {
	return xerrors.New("oops")
}

func (g badGatherer) Remove(tx txn.Transaction) error {
	return xerrors.New("oops")
}
