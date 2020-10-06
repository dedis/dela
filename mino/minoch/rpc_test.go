package minoch

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

func TestRPC_Call(t *testing.T) {
	manager := NewManager()

	mA := NewMinoch(manager, "A")
	rpcA, err := mA.MakeRPC("test", fakeHandler{}, fake.MessageFactory{})
	require.NoError(t, err)

	mB := NewMinoch(manager, "B")
	_, err = mB.MakeRPC("test", fakeHandler{}, fake.MessageFactory{})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrs := mino.NewAddresses(mA.GetAddress(), mB.GetAddress())
	resps, err := rpcA.Call(ctx, fake.Message{}, addrs)
	require.NoError(t, err)

	err = testWait(t, resps, nil)
	require.NoError(t, err)

	err = testWait(t, resps, nil)
	require.NoError(t, err)
}

func TestRPC_Filter_Call(t *testing.T) {
	manager := NewManager()

	m := NewMinoch(manager, "A")

	rpc, err := m.MakeRPC("test", fakeHandler{}, fake.MessageFactory{})
	require.NoError(t, err)

	m.AddFilter(func(m mino.Request) bool { return false })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resps, err := rpc.Call(ctx, fake.Message{}, mino.NewAddresses(m.GetAddress()))
	require.NoError(t, err)

	_, more := <-resps
	require.False(t, more)
}

func TestRPC_BadContext_Call(t *testing.T) {
	rpc := &RPC{
		context: fake.NewBadContext(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := rpc.Call(ctx, fake.Message{}, nil)
	require.EqualError(t, err, fake.Err("couldn't serialize"))
}

func TestRPC_BadFactory_Call(t *testing.T) {
	manager := NewManager()

	m := NewMinoch(manager, "A")

	rpc, err := m.MakeRPC("test", fakeHandler{}, fake.NewBadMessageFactory())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resps, err := rpc.Call(ctx, fake.Message{}, mino.NewAddresses(m.GetAddress()))
	require.NoError(t, err)

	err = testWait(t, resps, nil)
	require.EqualError(t, err, fake.Err("couldn't deserialize"))
}

func TestRPC_UnkownPeer_Call(t *testing.T) {
	manager := NewManager()

	m := NewMinoch(manager, "A")
	rpc, err := m.MakeRPC("test", fakeHandler{}, fake.MessageFactory{})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	to := address{
		id: "B",
	}

	_, err = rpc.Call(ctx, fake.Message{}, mino.NewAddresses(to))
	require.EqualError(t, err, "couldn't find peer: address <B> not found")
}

func TestRPC_MissingHandler_Call(t *testing.T) {
	manager := NewManager()

	mA := NewMinoch(manager, "A")
	rpcA, err := mA.MakeRPC("test", fakeHandler{}, fake.MessageFactory{})
	require.NoError(t, err)

	mB := NewMinoch(manager, "B")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resps, err := rpcA.Call(ctx, fake.Message{}, mino.NewAddresses(mB.GetAddress()))
	require.NoError(t, err)

	err = testWait(t, resps, nil)
	require.EqualError(t, err, "unknown rpc /test")
}

func TestRPC_BadHandler_Call(t *testing.T) {
	manager := NewManager()

	mA := NewMinoch(manager, "A")
	rpcA, err := mA.MakeRPC("test", fakeHandler{}, fake.MessageFactory{})
	require.NoError(t, err)

	mB := NewMinoch(manager, "B")
	_, err = mB.MakeRPC("test", badHandler{}, fake.MessageFactory{})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resps, err := rpcA.Call(ctx, fake.Message{}, mino.NewAddresses(mB.GetAddress()))
	require.NoError(t, err)

	err = testWait(t, resps, nil)
	require.EqualError(t, err, "couldn't process request: rpc is not supported")
}

func TestRPC_Stream(t *testing.T) {
	manager := NewManager()

	m := NewMinoch(manager, "A")
	rpc, err := m.MakeRPC("test", fakeStreamHandler{}, fake.MessageFactory{})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sender, receiver, err := rpc.Stream(ctx, mino.NewAddresses(m.GetAddress()))
	require.NoError(t, err)

	sender.Send(fake.Message{}, m.GetAddress())
	_, _, err = receiver.Recv(context.Background())
	require.NoError(t, err)

	ctx, cancel2 := context.WithCancel(context.Background())
	cancel2() // fake a timeout
	_, _, err = receiver.Recv(ctx)
	require.Equal(t, err, context.Canceled)
}

func TestRPC_Failures_Stream(t *testing.T) {
	manager := NewManager()

	m := NewMinoch(manager, "A")

	m.context = fake.NewBadContext()
	rpc, err := m.MakeRPC("test", fakeBadStreamHandler{}, fake.MessageFactory{})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, in, err := rpc.Stream(ctx, mino.NewAddresses(m.GetAddress()))
	require.NoError(t, err)
	_, _, err = in.Recv(ctx)
	require.EqualError(t, err, fake.Err("couldn't process"))

	errs := out.Send(fake.Message{})
	err = testWait(t, nil, errs)
	require.EqualError(t, err, fake.Err("couldn't marshal message"))

	m.context = serde.NewContext(fake.ContextEngine{})
	_, _, err = rpc.Stream(ctx, mino.NewAddresses(fake.NewAddress(0)))
	require.EqualError(t, err,
		"couldn't find peer: invalid address type 'fake.Address'")
}

func TestReceiver_Recv(t *testing.T) {
	recv := receiver{
		out:     make(chan Envelope, 1),
		errs:    make(chan error),
		context: serde.NewContext(fake.ContextEngine{}),
		factory: fake.MessageFactory{},
	}

	recv.out <- Envelope{
		from:    address{id: "A"},
		message: []byte(`{}`),
	}

	from, msg, err := recv.Recv(context.Background())
	require.NoError(t, err)
	require.Equal(t, address{id: "A"}, from)
	require.NotNil(t, msg)

	recv.factory = fake.NewBadMessageFactory()
	recv.out <- Envelope{}
	_, _, err = recv.Recv(context.Background())
	require.EqualError(t, err, fake.Err("couldn't deserialize"))
}

// -----------------------------------------------------------------------------
// Utility functions

func testWait(t *testing.T, resps <-chan mino.Response, errs <-chan error) error {
	select {
	case <-time.After(50 * time.Millisecond):
		t.Fatal("an error is expected")
		return nil
	case resp, more := <-resps:
		if !more {
			return nil
		}

		_, err := resp.GetMessageOrError()

		return err
	case err := <-errs:
		return err
	}
}

type fakeStreamHandler struct {
	mino.UnsupportedHandler
}

func (h fakeStreamHandler) Stream(out mino.Sender, in mino.Receiver) error {
	for {
		addr, msg, err := in.Recv(context.Background())
		if err != nil {
			return err
		}

		errs := out.Send(msg, addr)
		for err := range errs {
			return err
		}
	}
}

type fakeBadStreamHandler struct {
	mino.UnsupportedHandler
}

func (h fakeBadStreamHandler) Stream(out mino.Sender, in mino.Receiver) error {
	return fake.GetError()
}
