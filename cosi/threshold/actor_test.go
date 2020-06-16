package threshold

import (
	"context"
	"io"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde/tmp"
	"golang.org/x/xerrors"
)

func TestActor_Sign(t *testing.T) {
	ca := fake.NewAuthority(3, fake.NewSigner)

	actor := thresholdActor{
		CoSi: &CoSi{
			signer:    ca.GetSigner(0),
			Threshold: func(n int) int { return n - 1 },
		},
		rpc: fakeRPC{
			receiver: &fakeReceiver{
				resps: [][]interface{}{
					{ca.GetAddress(0), tmp.ProtoOf(fake.Message{})},
					{ca.GetAddress(0), tmp.ProtoOf(fake.Message{})},
					{ca.GetAddress(1), tmp.ProtoOf(fake.Message{})},
				},
			},
		},
		reactor: fakeReactor{},
	}

	ctx := context.Background()

	sig, err := actor.Sign(ctx, fake.Message{}, ca)
	require.NoError(t, err)
	require.NotNil(t, sig)

	actor.reactor = fakeReactor{err: xerrors.New("oops")}
	_, err = actor.Sign(ctx, fake.Message{}, ca)
	require.EqualError(t, err, "couldn't react to message: oops")

	actor.reactor = fakeReactor{}
	actor.rpc = fakeRPC{receiver: &fakeReceiver{}}
	_, err = actor.Sign(ctx, fake.Message{}, ca)
	require.EqualError(t, err, "couldn't receive more messages: EOF")

	actor.rpc = fakeRPC{receiver: &fakeReceiver{blocking: true}}
	doneCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = actor.Sign(doneCtx, fake.Message{}, ca)
	require.EqualError(t, err, "couldn't receive more messages: context canceled")

	actor.rpc = fakeRPC{sender: fakeSender{numErr: 2}, receiver: &fakeReceiver{blocking: true}}
	_, err = actor.Sign(ctx, fake.Message{}, ca)
	require.EqualError(t, err, "couldn't receive more messages: context canceled")

	actor.signer = fake.NewSigner()
	err = actor.merge(&Signature{}, fake.Signature{}, 0, fake.NewInvalidPublicKey(), []byte{})
	require.EqualError(t, err, "couldn't verify: fake error")

	actor.rpc = fake.NewBadStreamRPC()
	_, err = actor.Sign(ctx, fake.Message{}, ca)
	require.EqualError(t, err, "couldn't open stream: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeSender struct {
	mino.Sender
	numErr int
}

func (s fakeSender) Send(proto.Message, ...mino.Address) <-chan error {
	ch := make(chan error, s.numErr)
	for i := 0; i < s.numErr; i++ {
		ch <- xerrors.New("oops")
	}

	close(ch)
	return ch
}

type fakeReceiver struct {
	mino.Receiver
	blocking bool
	resps    [][]interface{}
	err      error
}

func (r *fakeReceiver) Recv(ctx context.Context) (mino.Address, proto.Message, error) {
	if r.blocking {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}

	if r.err != nil {
		return nil, nil, r.err
	}

	if len(r.resps) == 0 {
		return nil, nil, io.EOF
	}

	next := r.resps[0]
	r.resps = r.resps[1:]
	return next[0].(mino.Address), next[1].(proto.Message), nil
}

type fakeRPC struct {
	mino.RPC
	sender   fakeSender
	receiver *fakeReceiver
}

func (rpc fakeRPC) Stream(context.Context, mino.Players) (mino.Sender, mino.Receiver, error) {
	return rpc.sender, rpc.receiver, nil
}
