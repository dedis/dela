package skipchain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestHandler_Process(t *testing.T) {
	reactor := &fakeReactor{}
	watcher := &fakeWatcher{}
	h := newHandler(&operations{
		blockFactory: NewBlockFactory(fake.MessageFactory{}),
		db:           &fakeDatabase{},
		watcher:      watcher,
		reactor:      reactor,
	})

	genesis := SkipBlock{
		Index:   0,
		Payload: fake.Message{},
	}

	req := mino.Request{
		Message: PropagateGenesis{genesis: genesis},
	}
	resp, err := h.Process(req)
	require.NoError(t, err)
	require.Nil(t, resp)
	require.Len(t, reactor.calls, 1)
	require.Equal(t, genesis.Payload, reactor.calls[0][0])
	require.Equal(t, 1, watcher.notified)

	req.Message = fake.Message{}
	_, err = h.Process(req)
	require.EqualError(t, err, "unknown message type 'fake.Message'")

	reactor.errCommit = xerrors.New("oops")
	req.Message = PropagateGenesis{genesis: genesis}
	_, err = h.Process(req)
	require.EqualError(t, err,
		"couldn't store genesis: tx failed: couldn't commit block: oops")
}

func TestHandler_Stream(t *testing.T) {
	db := &fakeDatabase{blocks: []SkipBlock{
		{Payload: fake.Message{}},
		{hash: Digest{0x01}, Index: 1, Payload: fake.Message{}}},
	}
	h := handler{
		operations: &operations{
			db: db,
		},
	}

	rcvr := fakeReceiver{msg: BlockRequest{to: 1}}
	call := &fake.Call{}
	sender := fakeSender{call: call}

	err := h.Stream(sender, rcvr)
	require.NoError(t, err)
	require.Equal(t, 2, call.Len())

	err = h.Stream(sender, fakeReceiver{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't receive message: oops")

	err = h.Stream(sender, fakeReceiver{msg: fake.Message{}})
	require.EqualError(t, err,
		"invalid message type 'fake.Message' != 'skipchain.BlockRequest'")

	db.err = xerrors.New("oops")
	err = h.Stream(sender, rcvr)
	require.EqualError(t, err, "couldn't read block at index 0: oops")

	db.err = nil
	err = h.Stream(fakeSender{err: xerrors.New("oops")}, rcvr)
	require.EqualError(t, err, "couldn't send block: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeReceiver struct {
	mino.Receiver
	msg serde.Message
	err error
}

func (rcvr fakeReceiver) Recv(context.Context) (mino.Address, serde.Message, error) {
	return nil, rcvr.msg, rcvr.err
}

type fakeSender struct {
	mino.Sender
	call *fake.Call
	err  error
}

func (s fakeSender) Send(msg serde.Message, addrs ...mino.Address) <-chan error {
	s.call.Add(msg, addrs)
	errs := make(chan error, 1)
	errs <- s.err
	close(errs)
	return errs
}
