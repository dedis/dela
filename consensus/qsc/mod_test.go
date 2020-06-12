package qsc

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus"
	internal "go.dedis.ch/dela/internal/testing"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minoch"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&Message{},
		&MessageSet{},
		&View{},
		&RequestMessageSet{},
		&Epoch{},
		&History{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestQSC_Basic(t *testing.T) {
	n := 5
	k := 100

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
	require.GreaterOrEqual(t, len(cons[0].history), k)
}

func TestQSC_Listen(t *testing.T) {
	bc := &fakeBroadcast{wait: true, waiting: make(chan struct{})}
	qsc := &Consensus{
		closing:          make(chan struct{}),
		stopped:          make(chan struct{}),
		broadcast:        bc,
		historiesFactory: &fakeFactory{},
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
	factory := &fakeFactory{}
	qsc := &Consensus{
		broadcast:        bc,
		historiesFactory: factory,
	}

	ctx := context.Background()

	err := qsc.executeRound(ctx, fake.Message{}, badReactor{})
	require.EqualError(t, err, "couldn't validate proposal: oops")

	bc.err = xerrors.New("oops")
	err = qsc.executeRound(ctx, fake.Message{}, &fakeReactor{})
	require.EqualError(t, err, "couldn't broadcast: oops")

	bc.delay = 1
	factory.err = xerrors.New("oops")
	err = qsc.executeRound(ctx, fake.Message{}, &fakeReactor{})
	require.EqualError(t, err, "couldn't decode broadcasted set: oops")

	bc.delay = 1
	factory.delay = 1
	err = qsc.executeRound(ctx, fake.Message{}, &fakeReactor{})
	require.EqualError(t, err, "couldn't broadcast: oops")

	bc.err = nil
	factory.delay = 1
	err = qsc.executeRound(ctx, fake.Message{}, &fakeReactor{})
	require.EqualError(t, err, "couldn't decode received set: oops")

	factory.delay = 2
	err = qsc.executeRound(ctx, fake.Message{}, &fakeReactor{})
	require.EqualError(t, err, "couldn't decode broadcasted set: oops")

	factory.delay = 3
	err = qsc.executeRound(ctx, fake.Message{}, &fakeReactor{})
	require.EqualError(t, err, "couldn't decode received set: oops")

	factory.err = nil
	err = qsc.executeRound(ctx, nil, badReactor{})
	require.EqualError(t, err, "couldn't commit: oops")
}

// -----------------
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

func (v *fakeReactor) InvokeValidate(addr mino.Address, pb serde.Message) ([]byte, error) {
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

func (v badReactor) InvokeValidate(mino.Address, serde.Message) ([]byte, error) {
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

func (b *fakeBroadcast) send(ctx context.Context, h history) (*View, error) {
	if b.wait {
		close(b.waiting)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if b.delay == 0 {
		return nil, b.err
	}
	b.delay--
	return nil, nil
}

type fakeFactory struct {
	historiesFactory
	err   error
	delay int
}

func (f *fakeFactory) FromMessageSet(map[int64]*Message) (histories, error) {
	if f.delay == 0 && f.err != nil {
		return nil, f.err
	}
	f.delay--

	h := history{{random: 1}}
	return histories{h}, nil
}
