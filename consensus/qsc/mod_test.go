package qsc

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/consensus/qsc/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minoch"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

func TestQSC_Basic(t *testing.T) {
	n := 5
	k := 10

	cons := makeQSC(t, n)
	actors := make([]consensus.Actor, n)
	validators := make([]*fakeReactor, n)
	for i, c := range cons {
		val := &fakeReactor{count: 0, max: k}
		val.wg.Add(1)
		validators[i] = val

		actor, err := c.Listen(val)
		require.NoError(t, err)

		actors[i] = actor
	}

	for j := 0; j < n; j++ {
		actor := actors[j]
		go func() {
			for i := 0; i < k; i++ {
				err := actor.Propose(fake.Message{})
				require.NoError(t, err)
			}
		}()
	}

	for _, val := range validators {
		val.wg.Wait()
	}

	for _, actor := range actors {
		actor.Close()
	}

	require.Equal(t, cons[0].history, cons[1].history)
	require.GreaterOrEqual(t, len(cons[0].history.GetEpochs()), k)
}

func TestQSC_Listen(t *testing.T) {
	bc := &fakeBroadcast{wait: true, waiting: make(chan struct{})}
	qsc := &Consensus{
		closing:   make(chan struct{}),
		stopped:   make(chan struct{}),
		broadcast: bc,
	}

	actor, err := qsc.Listen(nil)
	require.NoError(t, err)

	select {
	case <-bc.waiting:
	case <-time.After(10 * time.Millisecond):
		t.Fatal("timeout")
	}
	require.NoError(t, actor.Close())

	// Make sure the Go routine is stopped.
	select {
	case <-qsc.stopped:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout")
	}
}

func TestQSC_ExecuteRound(t *testing.T) {
	bc := &fakeBroadcast{}
	qsc := &Consensus{
		broadcast: bc,
	}

	ctx := context.Background()

	err := qsc.executeRound(ctx, fake.Message{}, badReactor{})
	require.EqualError(t, err, "couldn't validate proposal: oops")

	bc.err = xerrors.New("oops")
	err = qsc.executeRound(ctx, fake.Message{}, &fakeReactor{})
	require.EqualError(t, err, "couldn't broadcast: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeQSC(t *testing.T, n int) []*Consensus {
	manager := minoch.NewManager()
	cons := make([]*Consensus, n)
	players := &fakePlayers{}
	for i := range cons {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)

		players.addrs = append(players.addrs, m.GetAddress())

		qsc, err := NewQSC(int64(i), m, players)
		require.NoError(t, err)

		cons[i] = qsc
	}

	return cons
}

type fakeReactor struct {
	consensus.Reactor
	count int
	max   int
	wg    sync.WaitGroup
}

func (v *fakeReactor) InvokeValidate(addr mino.Address, pb serdeng.Message) ([]byte, error) {
	return []byte{0xac}, nil
}

func (v *fakeReactor) InvokeCommit(id []byte) error {
	v.count++
	if v.count == v.max {
		v.wg.Done()
	}
	return nil
}

type badReactor struct {
	consensus.Reactor
}

func (v badReactor) InvokeValidate(mino.Address, serdeng.Message) ([]byte, error) {
	return nil, xerrors.New("oops")
}

func (v badReactor) InvokeCommit([]byte) error {
	return xerrors.New("oops")
}

type fakeBroadcast struct {
	broadcast
	err     error
	delay   int
	wait    bool
	waiting chan struct{}
}

func (b *fakeBroadcast) send(ctx context.Context, h types.History) (View, error) {
	if b.wait {
		close(b.waiting)
		<-ctx.Done()
		return View{}, ctx.Err()
	}
	if b.delay == 0 {
		return View{}, b.err
	}
	b.delay--
	return View{}, nil
}
