package qsc

import (
	"bytes"
	"context"
	fmt "fmt"
	"sync"
	"testing"
	"time"

	proto "github.com/golang/protobuf/proto"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minoch"
	"golang.org/x/xerrors"
)

func TestTLCR_Basic(t *testing.T) {
	n := 5
	k := 5
	bcs := makeTLCR(t, n)

	wg := sync.WaitGroup{}
	wg.Add(n)
	for _, bc := range bcs {
		go func(bc *bTLCR) {
			for i := 0; i < k; i++ {
				bc.execute(context.Background(), &Message{Node: bc.node})
			}
			wg.Done()
		}(bc)
	}

	wg.Wait()

	require.Equal(t, uint64(k), bcs[0].timeStep)
}

func TestTLCR_Execute(t *testing.T) {
	buffer := new(bytes.Buffer)
	ch := make(chan *MessageSet, 1)
	bc := &bTLCR{
		logger:  zerolog.New(buffer),
		rpc:     fakeRPC{},
		ch:      ch,
		players: fakeSinglePlayer{},
		store:   &storage{},
	}

	ch <- &MessageSet{
		Messages: map[int64]*Message{1: {}},
		TimeStep: 0,
	}

	view, err := bc.execute(context.Background())
	require.NoError(t, err)
	require.NotNil(t, view)
	require.Equal(t, uint64(1), bc.timeStep)
	require.Equal(t, bc.store.previous.GetMessages(), view.GetReceived())
	require.Len(t, view.GetBroadcasted(), 0)

	bc.rpc = fakeRPC{err: xerrors.New("oops")}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = bc.execute(ctx)
	require.EqualError(t, err, "context is done: context deadline exceeded")
	require.Contains(t, buffer.String(), "oops")
}

func TestTLCR_Merge(t *testing.T) {
	m1 := &MessageSet{Messages: map[int64]*Message{1: {}, 2: {}}}
	m2 := &MessageSet{Messages: map[int64]*Message{2: {}, 3: {}}}

	bc := &bTLCR{}
	bc.merge(m1, m2)
	require.Len(t, m1.GetMessages(), 3)
}

func TestTLCR_CatchUp(t *testing.T) {
	ch := make(chan *MessageSet, 1)
	bc := &bTLCR{
		ch:       ch,
		timeStep: 0,
		rpc:      fakeRPC{},
		players:  fakeSinglePlayer{},
	}

	m1 := &MessageSet{Messages: map[int64]*Message{1: {}}}
	m2 := &MessageSet{Node: 2}
	err := bc.catchUp(m1, m2)
	require.NoError(t, err)
	require.Equal(t, m2, <-ch)

	m2.TimeStep = 1
	bc.rpc = fakeRPC{msg: &MessageSet{Messages: map[int64]*Message{2: {}}}}
	err = bc.catchUp(m1, m2)
	require.NoError(t, err)
	require.Equal(t, m2, <-ch)

	bc.rpc = fakeRPC{msg: &empty.Empty{}}
	err = bc.catchUp(m1, m2)
	require.EqualError(t, err, "couldn't fetch previous message set: invalid message type: *empty.Empty")
	require.Equal(t, m2, <-ch)

	bc.rpc = fakeRPC{err: xerrors.New("oops")}
	err = bc.catchUp(m1, m2)
	require.EqualError(t, err, "couldn't fetch previous message set: couldn't reach the node: oops")
	require.Equal(t, m2, <-ch)

	bc.rpc = fakeRPC{closed: true}
	err = bc.catchUp(m1, m2)
	require.EqualError(t, err, "couldn't fetch previous message set: couldn't get a reply")
	require.Equal(t, m2, <-ch)
}

func TestTLCB_Basic(t *testing.T) {
	n := 3
	k := 5

	bcs := makeTLCB(t, n)
	wg := sync.WaitGroup{}
	wg.Add(n)
	for _, bc := range bcs {
		go func(bc *bTLCB) {
			defer wg.Done()
			var view *View
			var err error
			for i := 0; i < k; i++ {
				view, err = bc.execute(context.Background(), &empty.Empty{})
				require.NoError(t, err)
				require.Len(t, view.GetBroadcasted(), n)
				require.Len(t, view.GetReceived(), n)
			}
		}(bc)
	}

	wg.Wait()
}

func TestTLCB_Execute(t *testing.T) {
	defer func() { protoenc = encoding.NewProtoEncoder() }()

	bc := &bTLCB{
		b1: fakeTLCR{},
		b2: fakeTLCR{},
	}

	view, err := bc.execute(context.Background(), &empty.Empty{})
	require.NoError(t, err)
	require.NotNil(t, view)

	bc.b1 = fakeTLCR{err: xerrors.New("oops")}
	_, err = bc.execute(context.Background(), &empty.Empty{})
	require.EqualError(t, err, "couldn't broadcast: oops")

	bc.b1 = fakeTLCR{}
	bc.b2 = fakeTLCR{err: xerrors.New("oops")}
	_, err = bc.execute(context.Background(), &empty.Empty{})
	require.EqualError(t, err, "couldn't broadcast: oops")

	bc.b2 = fakeTLCR{}
	protoenc = &fakeEncoder{}
	_, err = bc.execute(context.Background(), &empty.Empty{})
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewAnyEncodingError((*empty.Empty)(nil), nil)))

	protoenc = &fakeEncoder{delay: 1}
	_, err = bc.execute(context.Background(), &empty.Empty{})
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewAnyEncodingError((*MessageSet)(nil), nil)))
}

func makeTLCR(t *testing.T, n int) []*bTLCR {
	manager := minoch.NewManager()
	players := &fakePlayers{}
	bcs := make([]*bTLCR, n)
	for i := range bcs {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)

		players.addrs = append(players.addrs, m.GetAddress())

		bc, err := newTLCR("tlcr", int64(i), m, players)
		require.NoError(t, err)

		bcs[i] = bc
	}

	return bcs
}

func makeTLCB(t *testing.T, n int) []*bTLCB {
	manager := minoch.NewManager()
	players := &fakePlayers{}
	bcs := make([]*bTLCB, n)
	for i := range bcs {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)

		players.addrs = append(players.addrs, m.GetAddress())

		bc, err := newTLCB(int64(i), m, players)
		require.NoError(t, err)

		bcs[i] = bc
	}

	return bcs
}

// -----------------
// Utility functions

type fakeIterator struct {
	index int
	addrs []mino.Address
}

func (i *fakeIterator) HasNext() bool {
	return i.index+1 < len(i.addrs)
}

func (i *fakeIterator) GetNext() mino.Address {
	if i.HasNext() {
		i.index++
		return i.addrs[i.index]
	}
	return nil
}

type fakePlayers struct {
	addrs []mino.Address
}

func (p *fakePlayers) SubSet(from, to int) mino.Players {
	return &fakePlayers{addrs: p.addrs[from:to]}
}

func (p *fakePlayers) AddressIterator() mino.AddressIterator {
	return &fakeIterator{addrs: p.addrs, index: -1}
}

func (p *fakePlayers) Len() int {
	return len(p.addrs)
}

type fakeSinglePlayer struct {
	mino.Players
}

func (p fakeSinglePlayer) Len() int {
	return 1
}

func (p fakeSinglePlayer) SubSet(from, to int) mino.Players {
	return fakeSinglePlayer{}
}

type fakeRPC struct {
	mino.RPC
	err    error
	msg    proto.Message
	closed bool
}

func (rpc fakeRPC) Call(pb proto.Message, players mino.Players) (<-chan proto.Message, <-chan error) {
	errs := make(chan error, 1)
	if rpc.err != nil {
		errs <- rpc.err
	}
	msgs := make(chan proto.Message, 1)
	if rpc.msg != nil {
		msgs <- rpc.msg
	}
	if rpc.closed {
		close(msgs)
	}
	return msgs, errs
}

type fakeTLCR struct {
	err error
}

func (b fakeTLCR) execute(context.Context, ...*Message) (*View, error) {
	return nil, b.err
}

type fakeEncoder struct {
	encoding.ProtoEncoder
	delay int
}

func (e *fakeEncoder) MarshalAny(pb proto.Message) (*any.Any, error) {
	if e.delay == 0 {
		return nil, xerrors.New("oops")
	}
	e.delay--
	return nil, nil
}
