package minoch

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestRPC_Call(t *testing.T) {
	manager := NewManager()

	m1, err := NewMinoch(manager, "A")
	require.NoError(t, err)
	rpc1, err := m1.MakeRPC("test", fakeHandler{}, fake.MessageFactory{})
	require.NoError(t, err)

	m2, err := NewMinoch(manager, "B")
	require.NoError(t, err)
	_, err = m2.MakeRPC("test", badHandler{}, fake.MessageFactory{})
	require.NoError(t, err)

	m3, err := NewMinoch(manager, "C")
	require.NoError(t, err)

	ctx := context.Background()

	addrs := mino.NewAddresses(m1.GetAddress())
	resps, errs := rpc1.Call(ctx, fake.Message{}, addrs)

	err = testWait(t, errs, resps)
	// Message to self with a correct handler
	require.NoError(t, err)

	addrs = mino.NewAddresses(fake.NewAddress(99), m3.GetAddress())
	resps, errs = rpc1.Call(ctx, fake.Message{}, addrs)

	err = testWait(t, errs, resps)
	// Message to the fake address.
	require.EqualError(t, err,
		"couldn't find peer: invalid address type 'fake.Address'")

	err = testWait(t, errs, resps)
	// Message to m3 that has not the handler registered.
	require.EqualError(t, err, "unknown rpc /test")

	addrs = mino.NewAddresses(m2.GetAddress())
	resps, errs = rpc1.Call(ctx, fake.Message{}, addrs)

	err = testWait(t, errs, resps)
	// Message to m2 with a handler but no implementation.
	require.EqualError(t, err, "couldn't process request: rpc is not supported")
}

func TestRPC_Stream(t *testing.T) {
	manager := NewManager()

	m, err := NewMinoch(manager, "A")
	require.NoError(t, err)
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
	require.EqualError(t, err, "timeout")
}

func TestRPC_Failures_Stream(t *testing.T) {
	manager := NewManager()

	m, err := NewMinoch(manager, "A")
	require.NoError(t, err)

	m.context = fake.NewBadContext()
	rpc, err := m.MakeRPC("test", fakeBadStreamHandler{}, fake.MessageFactory{})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, in, err := rpc.Stream(ctx, mino.NewAddresses(m.GetAddress()))
	require.NoError(t, err)
	_, _, err = in.Recv(ctx)
	require.EqualError(t, err, "couldn't process: oops")

	errs := out.Send(fake.Message{})
	err = testWait(t, errs, nil)
	require.EqualError(t, err, "couldn't marshal message: fake error")

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
	require.EqualError(t, err, "couldn't deserialize: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

func testWait(t *testing.T, errs <-chan error, resps <-chan serde.Message) error {
	select {
	case <-time.After(50 * time.Millisecond):
		t.Fatal("an error is expected")
		return nil
	case <-resps:
		return nil
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
	return xerrors.New("oops")
}
