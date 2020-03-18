package minoch

import (
	"context"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

func TestRPC_Call(t *testing.T) {
	manager := NewManager()

	m1, err := NewMinoch(manager, "A")
	require.NoError(t, err)
	rpc1, err := m1.MakeRPC("test", testHandler{})
	require.NoError(t, err)

	m2, err := NewMinoch(manager, "B")
	require.NoError(t, err)
	_, err = m2.MakeRPC("test", testHandler{})
	require.NoError(t, err)

	resps, errs := rpc1.Call(&empty.Empty{}, fakeMembership{instances: []*Minoch{m2}})
	select {
	case <-resps:
		t.Fatal("an error is expected")
	case <-errs:
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

func TestRPC_Stream(t *testing.T) {
	manager := NewManager()

	m, err := NewMinoch(manager, "A")
	require.NoError(t, err)
	rpc, err := m.MakeRPC("test", fakeStreamHandler{})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sender, receiver := rpc.Stream(ctx, fakeMembership{instances: []*Minoch{m}})

	sender.Send(&empty.Empty{}, m.GetAddress())
	_, _, err = receiver.Recv(context.Background())
	require.NoError(t, err)

	ctx, cancel2 := context.WithCancel(context.Background())
	cancel2() // fake a timeout
	_, _, err = receiver.Recv(ctx)
	require.Error(t, err)
}

func TestRPC_StreamFailures(t *testing.T) {
	manager := NewManager()

	m, err := NewMinoch(manager, "A")
	require.NoError(t, err)
	rpc, err := m.MakeRPC("test", fakeBadStreamHandler{})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, in := rpc.Stream(ctx, fakeMembership{instances: []*Minoch{m}})
	_, _, err = in.Recv(ctx)
	require.Error(t, err)
	errs := out.Send(nil)
	select {
	case _, ok := <-errs:
		if !ok {
			t.Error("expected an error")
		}
	}
}

type fakeIterator struct {
	instances []*Minoch
	index     int
}

func (i *fakeIterator) HasNext() bool {
	if i.index+1 < len(i.instances) {
		return true
	}
	return false
}

func (i *fakeIterator) GetNext() mino.Address {
	if i.HasNext() {
		i.index++
		return i.instances[i.index].GetAddress()
	}
	return nil
}

type fakeMembership struct {
	instances []*Minoch
}

func (m fakeMembership) AddressIterator() mino.AddressIterator {
	return &fakeIterator{
		index:     -1,
		instances: m.instances,
	}
}

func (m fakeMembership) Len() int {
	return len(m.instances)
}
