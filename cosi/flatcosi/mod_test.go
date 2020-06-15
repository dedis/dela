package flatcosi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde/tmp"
	"golang.org/x/xerrors"
)

func TestFlat_GetSigner(t *testing.T) {
	flat := NewFlat(nil, fake.NewSigner())
	require.NotNil(t, flat.GetSigner())
}

func TestFlat_GetPublicKeyFactory(t *testing.T) {
	flat := NewFlat(nil, fake.NewSigner())
	require.NotNil(t, flat.GetPublicKeyFactory())
}

func TestFlat_GetSignatureFactory(t *testing.T) {
	flat := NewFlat(nil, fake.NewSigner())
	require.NotNil(t, flat.GetSignatureFactory())
}

func TestFlat_GetVerifierFactory(t *testing.T) {
	flat := NewFlat(nil, fake.NewSigner())
	require.NotNil(t, flat.GetVerifierFactory())
}

func TestFlat_Listen(t *testing.T) {
	flat := NewFlat(fake.Mino{}, bls.NewSigner())

	a, err := flat.Listen(fakeReactor{})
	require.NoError(t, err)
	actor := a.(flatActor)
	require.NotNil(t, actor.signer)
	require.NotNil(t, actor.rpc)

	flat.mino = fake.NewBadMino()
	_, err = flat.Listen(fakeReactor{})
	require.EqualError(t, err, "couldn't make the rpc: fake error")
}

func TestActor_Sign(t *testing.T) {
	message := fake.Message{}
	ca := fake.NewAuthority(1, fake.NewSigner)

	rpc := fake.NewRPC()
	actor := flatActor{
		signer:  fake.NewSigner(),
		rpc:     rpc,
		reactor: fakeReactor{},
	}

	rpc.Msgs <- tmp.ProtoOf(SignatureResponse{signature: fake.Signature{}})
	rpc.Msgs <- tmp.ProtoOf(SignatureResponse{signature: fake.Signature{}})
	close(rpc.Msgs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig, err := actor.Sign(ctx, message, ca)
	require.NoError(t, err)
	require.NotNil(t, sig)

	actor.signer = fake.NewSignerWithVerifierFactory(fake.NewBadVerifierFactory())
	_, err = actor.Sign(ctx, message, ca)
	require.EqualError(t, err, "couldn't make verifier: fake error")

	actor.signer = fake.NewSigner()
	actor.reactor = fakeReactor{err: xerrors.New("oops")}
	_, err = actor.Sign(ctx, message, ca)
	require.EqualError(t, err, "couldn't react to message: oops")
}

func TestActor_SignWrongSignature(t *testing.T) {
	message := fake.Message{}
	ca := fake.NewAuthority(1, bls.NewSigner)

	rpc := fake.NewRPC()
	actor := flatActor{
		signer:  ca.GetSigner(0),
		rpc:     rpc,
		reactor: fakeReactor{},
	}

	rpc.Msgs <- tmp.ProtoOf(SignatureResponse{signature: fake.Signature{}})
	close(rpc.Msgs)

	ctx := context.Background()

	_, err := actor.Sign(ctx, message, ca)
	require.EqualError(t, err,
		"couldn't verify the aggregation: bn256.G1: not enough data")
}

func TestActor_RPCError_Sign(t *testing.T) {
	message := fake.Message{}
	ca := fake.NewAuthority(1, fake.NewSigner)

	rpc := fake.NewRPC()
	actor := flatActor{
		signer:  ca.GetSigner(0),
		rpc:     rpc,
		reactor: fakeReactor{},
	}

	close(rpc.Msgs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig, err := actor.Sign(ctx, message, ca)
	require.EqualError(t, err, "signature is nil")
	require.Nil(t, sig)
}

func TestActor_Context_Sign(t *testing.T) {
	message := fake.Message{}
	ca := fake.NewAuthority(1, fake.NewSigner)
	rpc := fake.NewRPC()

	actor := flatActor{
		signer:  ca.GetSigner(0),
		rpc:     rpc,
		reactor: fakeReactor{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rpc.Errs <- xerrors.New("oops")

	sig, err := actor.Sign(ctx, message, ca)
	require.EqualError(t, err, "one request has failed: oops")
	require.Nil(t, sig)
}

func TestActor_SignProcessError(t *testing.T) {
	ca := fake.NewAuthority(1, fake.NewSigner)

	actor := flatActor{
		signer:  ca.GetSigner(0),
		reactor: fakeReactor{},
	}

	_, err := actor.processResponse(fake.Message{}, nil)
	require.EqualError(t, err, "invalid response type 'fake.Message'")

	actor.signer = fake.NewBadSigner()
	_, err = actor.processResponse(SignatureResponse{}, fake.Signature{})
	require.EqualError(t, err, "couldn't aggregate: fake error")
}
