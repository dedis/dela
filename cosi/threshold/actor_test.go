package threshold

import (
	"context"
	"io"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

func TestActor_Sign(t *testing.T) {
	ca := fake.NewAuthority(3, fake.NewSigner)

	actor := thresholdActor{
		CoSi: &CoSi{
			encoder:   encoding.NewProtoEncoder(),
			signer:    ca.GetSigner(0),
			Threshold: func(n int) int { return n - 1 },
		},
		rpc: fakeRPC{
			receiver: &fakeReceiver{
				resps: [][]interface{}{
					{ca.GetAddress(0), &empty.Empty{}},
					{ca.GetAddress(0), &empty.Empty{}},
					{ca.GetAddress(1), &empty.Empty{}},
				},
			},
		},
	}

	ctx := context.Background()

	sig, err := actor.Sign(ctx, fakeMessage{}, ca)
	require.NoError(t, err)
	require.NotNil(t, sig)

	actor.CoSi.encoder = fake.BadPackEncoder{}
	_, err = actor.Sign(ctx, fakeMessage{}, ca)
	require.EqualError(t, err, "couldn't pack message: fake error")

	actor.CoSi.encoder = encoding.NewProtoEncoder()
	actor.rpc = fakeRPC{receiver: &fakeReceiver{}}
	_, err = actor.Sign(ctx, fakeMessage{}, ca)
	require.EqualError(t, err, "couldn't receive more messages: EOF")

	actor.rpc = fakeRPC{receiver: &fakeReceiver{blocking: true}}
	doneCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = actor.Sign(doneCtx, fakeMessage{}, ca)
	require.EqualError(t, err, "couldn't receive more messages: context canceled")

	actor.rpc = fakeRPC{sender: fakeSender{numErr: 2}, receiver: &fakeReceiver{blocking: true}}
	_, err = actor.Sign(ctx, fakeMessage{}, ca)
	require.EqualError(t, err, "couldn't receive more messages: context canceled")

	actor.signer = fake.NewSignerWithSignatureFactory(fake.NewBadSignatureFactory())
	err = actor.merge(&Signature{}, &empty.Empty{}, 0, nil, nil)
	require.EqualError(t, err, "couldn't decode signature: fake error")

	actor.signer = fake.NewSigner()
	err = actor.merge(&Signature{}, &empty.Empty{}, 0, fake.NewInvalidPublicKey(), fakeMessage{})
	require.EqualError(t, err, "couldn't verify: fake error")
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

func (rpc fakeRPC) Stream(context.Context, mino.Players) (mino.Sender, mino.Receiver) {
	return rpc.sender, rpc.receiver
}
