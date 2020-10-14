package gossip

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/gossip"
	"go.dedis.ch/dela/mino/minoch"
	"go.dedis.ch/dela/serde"
)

func TestPool_Basic(t *testing.T) {
	_, pools := makeRoster(t, 10)
	defer func() {
		for _, pool := range pools {
			require.NoError(t, pool.Close())
		}
	}()

	go func() {
		for i := 0; i < 50; i++ {
			err := pools[0].Add(makeTx(uint64(i)))
			require.NoError(t, err)
		}
	}()

	go func() {
		for i := 0; i < 50; i++ {
			err := pools[2].Add(makeTx(uint64(i + 50)))
			require.NoError(t, err)
		}
	}()

	go func() {
		for i := 0; i < 50; i++ {
			err := pools[7].Add(makeTx(uint64(i + 100)))
			require.NoError(t, err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	txs := pools[0].Gather(ctx, pool.Config{Min: 150})
	require.Len(t, txs, 150)
}

func TestPool_New(t *testing.T) {
	pool, err := NewPool(fakeGossiper{})
	require.NoError(t, err)
	require.NotNil(t, pool)

	err = pool.Close()
	require.NoError(t, err)

	_, err = NewPool(fakeGossiper{err: fake.GetError()})
	require.EqualError(t, err, fake.Err("failed to listen"))
}

func TestPool_Len(t *testing.T) {
	p := &Pool{
		gatherer: pool.NewSimpleGatherer(),
	}

	require.Equal(t, 0, p.Len())

	p.gatherer.Add(makeTx(0))
	require.Equal(t, 1, p.Len())
}

func TestPool_AddFilter(t *testing.T) {
	p := &Pool{
		gatherer: pool.NewSimpleGatherer(),
	}

	p.AddFilter(nil)
}

func TestPool_Add(t *testing.T) {
	p := &Pool{
		actor:    fakeActor{},
		gatherer: pool.NewSimpleGatherer(),
	}

	err := p.Add(makeTx(0))
	require.NoError(t, err)

	p.gatherer = badGatherer{}
	err = p.Add(makeTx(0))
	require.EqualError(t, err, fake.Err("store failed"))

	p.gatherer = pool.NewSimpleGatherer()
	p.actor = fakeActor{err: fake.GetError()}
	err = p.Add(makeTx(0))
	require.EqualError(t, err, fake.Err("failed to gossip tx"))
}

func TestPool_Remove(t *testing.T) {
	p := &Pool{
		actor:    fakeActor{},
		gatherer: pool.NewSimpleGatherer(),
	}

	tx := makeTx(0)

	require.NoError(t, p.gatherer.Add(tx))

	err := p.Remove(tx)
	require.NoError(t, err)

	p.gatherer = badGatherer{}
	err = p.Remove(tx)
	require.EqualError(t, err, fake.Err("store failed"))
}

func TestPool_Gather(t *testing.T) {
	p := &Pool{
		actor:    fakeActor{},
		gatherer: pool.NewSimpleGatherer(),
	}

	ctx := context.Background()

	cb := func() {
		require.NoError(t, p.Add(makeTx(0)))
		require.NoError(t, p.Add(makeTx(1)))
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
		gatherer: pool.NewSimpleGatherer(),
		closing:  make(chan struct{}),
		actor:    fakeActor{},
	}

	err := pool.Close()
	require.NoError(t, err)

	pool.closing = make(chan struct{})
	pool.actor = fakeActor{err: fake.GetError()}
	err = pool.Close()
	require.EqualError(t, err, fake.Err("failed to close gossiper"))
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
		ch <- makeTx(0)
		close(p.closing)
	}()

	p.listenRumors(ch)
	require.Empty(t, buffer.String())

	p.gatherer = badGatherer{}
	p.closing = make(chan struct{})

	ch = make(chan gossip.Rumor)
	go func() {
		ch <- makeTx(0)
		close(p.closing)
	}()

	p.listenRumors(ch)
	require.NotEmpty(t, buffer.String())
}

// -----------------------------------------------------------------------------
// Utility functions

func makeTx(nonce uint64) txn.Transaction {
	return fakeTx{nonce: nonce}
}

func makeRoster(t *testing.T, n int) (mino.Players, []*Pool) {
	manager := minoch.NewManager()

	pools := make([]*Pool, n)
	addrs := make([]mino.Address, n)

	for i := 0; i < n; i++ {
		m := minoch.MustCreate(manager, fmt.Sprintf("node%d", i))

		addrs[i] = m.GetAddress()

		g := gossip.NewFlat(m, fakeTxFac{})

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

type fakeTx struct {
	txn.Transaction

	nonce uint64
}

func (tx fakeTx) GetNonce() uint64 {
	return tx.nonce
}

func (tx fakeTx) GetIdentity() access.Identity {
	return fake.PublicKey{}
}

func (tx fakeTx) GetID() []byte {
	return []byte{byte(tx.nonce)}
}

func (tx fakeTx) Serialize(serde.Context) ([]byte, error) {
	return tx.GetID(), nil
}

type fakeTxFac struct {
	txn.Factory
}

func (fakeTxFac) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return fakeTx{nonce: uint64(data[0])}, nil
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
	return fake.GetError()
}

func (g badGatherer) Remove(tx txn.Transaction) error {
	return fake.GetError()
}
