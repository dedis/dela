package threshold

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestActor_Sign(t *testing.T) {
	roster := fake.NewAuthority(3, fake.NewSigner)

	recv := fake.NewReceiver(
		fake.NewRecvMsg(fake.NewAddress(0), cosi.SignatureResponse{Signature: fake.Signature{}}),
		fake.NewRecvMsg(fake.NewAddress(0), cosi.SignatureResponse{Signature: fake.Signature{}}),
		fake.NewRecvMsg(fake.NewAddress(1), cosi.SignatureResponse{Signature: fake.Signature{}}),
	)
	rpc := fake.NewStreamRPC(recv, fake.Sender{})

	actor := thresholdActor{
		Threshold: &Threshold{
			signer: roster.GetSigner(0).(crypto.AggregateSigner),
		},
		rpc:     rpc,
		reactor: fakeReactor{},
	}

	actor.SetThreshold(OneThreshold)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig, err := actor.Sign(ctx, fake.Message{}, roster)
	require.NoError(t, err)
	require.NotNil(t, sig)
}

func TestActor_BadNetwork_Sign(t *testing.T) {
	actor := thresholdActor{
		Threshold: &Threshold{},
		rpc:       fake.NewBadRPC(),
		reactor:   fakeReactor{err: fake.GetError()},
	}

	roster := fake.NewAuthority(3, fake.NewSigner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := actor.Sign(ctx, fake.Message{}, roster)
	require.EqualError(t, err, fake.Err("couldn't open stream"))
}

func TestActor_BadReactor_Sign(t *testing.T) {
	actor := thresholdActor{
		Threshold: &Threshold{},
		rpc:       fake.NewRPC(),
		reactor:   fakeReactor{err: fake.GetError()},
	}

	roster := fake.NewAuthority(3, fake.NewSigner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := actor.Sign(ctx, fake.Message{}, roster)
	require.EqualError(t, err, fake.Err("couldn't react to message"))
}

func TestActor_MalformedResponse_Sign(t *testing.T) {
	logger, check := fake.CheckLog("failed to process signature response")

	recv := fake.NewReceiver(fake.NewRecvMsg(fake.NewAddress(0), fake.Message{}))
	rpc := fake.NewStreamRPC(recv, fake.Sender{})
	rpc.Done()

	actor := thresholdActor{
		Threshold: &Threshold{
			logger: logger,
		},
		rpc:     rpc,
		reactor: fakeReactor{},
	}
	actor.thresholdFn.Store(cosi.Threshold(defaultThreshold))

	roster := fake.NewAuthority(3, fake.NewSigner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := actor.Sign(ctx, fake.Message{}, roster)
	require.EqualError(t, err, "couldn't receive more messages: EOF")
	check(t)
}

func TestActor_CanceledContext_Sign(t *testing.T) {
	actor := thresholdActor{
		Threshold: &Threshold{},
		rpc:       fake.NewStreamRPC(fake.NewBlockingReceiver(), fake.Sender{}),
		reactor:   fakeReactor{},
	}
	actor.thresholdFn.Store(cosi.Threshold(defaultThreshold))

	roster := fake.NewAuthority(3, fake.NewSigner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := actor.Sign(ctx, fake.Message{}, roster)
	require.EqualError(t, err, "couldn't receive more messages: context canceled")
}

func TestActor_TooManyErrors_Sign(t *testing.T) {
	rpc := fake.NewStreamRPC(fake.NewBlockingReceiver(), fake.NewBadSender())

	logger, check := fake.CheckLog("signature request to a peer failed")

	actor := thresholdActor{
		Threshold: &Threshold{
			logger: logger,
		},
		rpc:     rpc,
		reactor: fakeReactor{},
	}
	actor.thresholdFn.Store(cosi.Threshold(defaultThreshold))

	roster := fake.NewAuthority(3, fake.NewSigner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := actor.Sign(ctx, fake.Message{}, roster)
	require.EqualError(t, err, "couldn't receive more messages: context canceled")
	check(t)
}

func TestActor_InvalidSignature_Sign(t *testing.T) {
	recv := fake.NewReceiver(
		fake.NewRecvMsg(fake.NewAddress(0), cosi.SignatureResponse{Signature: fake.Signature{}}),
	)
	rpc := fake.NewStreamRPC(recv, fake.Sender{})

	logger, check := fake.CheckLog("failed to process signature response")

	actor := thresholdActor{
		Threshold: &Threshold{
			logger: logger,
		},
		rpc:     rpc,
		reactor: fakeReactor{},
	}
	actor.thresholdFn.Store(cosi.Threshold(defaultThreshold))

	roster := fake.NewAuthority(3, func() crypto.Signer {
		return fake.NewSignerWithPublicKey(fake.NewBadPublicKey())
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := actor.Sign(ctx, fake.Message{}, roster)
	require.EqualError(t, err, "couldn't receive more messages: EOF")
	check(t)
}
