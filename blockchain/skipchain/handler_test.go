package skipchain

import (
	"context"
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

func TestHandler_Process(t *testing.T) {
	proc := &fakePayloadProc{}
	watcher := &fakeWatcher{}
	h := newHandler(&operations{
		processor: proc,
		blockFactory: blockFactory{
			encoder:     encoding.NewProtoEncoder(),
			hashFactory: crypto.NewSha256Factory(),
		},
		db:      &fakeDatabase{},
		watcher: watcher,
	})

	genesis := SkipBlock{
		Index:   0,
		Payload: &wrappers.BoolValue{Value: true},
	}

	packed, err := genesis.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)

	req := mino.Request{
		Message: &PropagateGenesis{Genesis: packed.(*BlockProto)},
	}
	resp, err := h.Process(req)
	require.NoError(t, err)
	require.Nil(t, resp)
	require.Len(t, proc.calls, 2)
	require.Equal(t, uint64(0), proc.calls[0][0])
	require.True(t, proto.Equal(genesis.Payload, proc.calls[0][1].(proto.Message)))
	require.True(t, proto.Equal(genesis.Payload, proc.calls[1][0].(proto.Message)))
	require.Equal(t, 1, watcher.notified)

	req.Message = &empty.Empty{}
	_, err = h.Process(req)
	require.EqualError(t, err, "unknown message type '*empty.Empty'")

	req.Message = &PropagateGenesis{}
	_, err = h.Process(req)
	require.EqualError(t, err,
		"couldn't decode block: couldn't unmarshal payload: message is nil")

	proc.errValidate = xerrors.New("oops")
	req.Message = &PropagateGenesis{Genesis: packed.(*BlockProto)}
	_, err = h.Process(req)
	require.EqualError(t, err,
		"couldn't store genesis: couldn't validate block: oops")
}

func TestHandler_Stream(t *testing.T) {
	db := &fakeDatabase{blocks: []SkipBlock{
		{Payload: &empty.Empty{}},
		{hash: Digest{0x01}, Index: 1, Payload: &empty.Empty{}}},
	}
	h := handler{
		operations: &operations{
			encoder:   encoding.NewProtoEncoder(),
			db:        db,
			consensus: fakeConsensus{},
		},
	}

	rcvr := fakeReceiver{msg: &BlockRequest{To: 1}}
	call := &fake.Call{}
	sender := fakeSender{call: call}

	err := h.Stream(sender, rcvr)
	require.NoError(t, err)
	require.Equal(t, 2, call.Len())

	err = h.Stream(sender, fakeReceiver{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't receive message: oops")

	err = h.Stream(sender, fakeReceiver{msg: nil})
	require.EqualError(t, err,
		"invalid message type '<nil>' != '*skipchain.BlockRequest'")

	db.err = xerrors.New("oops")
	err = h.Stream(sender, rcvr)
	require.EqualError(t, err, "couldn't read block at index 0: oops")

	db.err = nil
	h.encoder = fake.BadPackEncoder{}
	err = h.Stream(sender, rcvr)
	require.EqualError(t, err, "couldn't pack block: fake error")

	h.encoder = encoding.NewProtoEncoder()
	h.consensus = fakeConsensus{err: xerrors.New("oops")}
	err = h.Stream(sender, rcvr)
	require.EqualError(t, err, "couldn't get chain to block 1: oops")

	h.consensus = fakeConsensus{}
	h.encoder = fake.BadPackAnyEncoder{}
	err = h.Stream(sender, rcvr)
	require.EqualError(t, err, "couldn't pack chain: fake error")

	h.encoder = encoding.NewProtoEncoder()
	err = h.Stream(fakeSender{err: xerrors.New("oops")}, rcvr)
	require.EqualError(t, err, "couldn't send block: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeReceiver struct {
	mino.Receiver
	msg proto.Message
	err error
}

func (rcvr fakeReceiver) Recv(context.Context) (mino.Address, proto.Message, error) {
	return nil, rcvr.msg, rcvr.err
}

type fakeSender struct {
	mino.Sender
	call *fake.Call
	err  error
}

func (s fakeSender) Send(msg proto.Message, addrs ...mino.Address) <-chan error {
	s.call.Add(msg, addrs)
	errs := make(chan error, 1)
	errs <- s.err
	close(errs)
	return errs
}
