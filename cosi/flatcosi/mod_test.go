package flatcosi

import (
	"context"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/encoding"
	internal "go.dedis.ch/dela/internal/testing"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&SignatureRequest{},
		&SignatureResponse{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

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
	message := fakeMessage{}
	ca := fake.NewAuthority(1, fake.NewSigner)

	rpc := fake.NewRPC()
	actor := flatActor{
		encoder: encoding.NewProtoEncoder(),
		signer:  fake.NewSigner(),
		rpc:     rpc,
		reactor: fakeReactor{},
	}

	sigAny := makePackedSignature(t, actor.signer, []byte{0xab})

	rpc.Msgs <- &SignatureResponse{Signature: sigAny}
	rpc.Msgs <- &SignatureResponse{Signature: sigAny}
	close(rpc.Msgs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig, err := actor.Sign(ctx, message, ca)
	require.NoError(t, err)
	require.NotNil(t, sig)

	actor.encoder = fake.BadPackAnyEncoder{}
	_, err = actor.Sign(ctx, message, nil)
	require.EqualError(t, err, "couldn't pack message: fake error")

	actor.encoder = encoding.NewProtoEncoder()
	actor.signer = fake.NewSignerWithVerifierFactory(fake.NewBadVerifierFactory())
	_, err = actor.Sign(ctx, message, ca)
	require.EqualError(t, err, "couldn't make verifier: fake error")
}

func TestActor_SignWrongSignature(t *testing.T) {
	message := fakeMessage{}
	ca := fake.NewAuthority(1, bls.NewSigner)

	rpc := fake.NewRPC()
	actor := flatActor{
		encoder: encoding.NewProtoEncoder(),
		signer:  ca.GetSigner(0),
		rpc:     rpc,
		reactor: fakeReactor{},
	}

	sigAny := makePackedSignature(t, ca.GetSigner(0), []byte{0xef})

	rpc.Msgs <- &SignatureResponse{Signature: sigAny}
	close(rpc.Msgs)

	ctx := context.Background()

	_, err := actor.Sign(ctx, message, ca)
	require.EqualError(t, err, "couldn't verify the aggregation: bls: invalid signature")
}

func TestActor_RPCError_Sign(t *testing.T) {
	message := fakeMessage{}
	ca := fake.NewAuthority(1, fake.NewSigner)

	rpc := fake.NewRPC()
	actor := flatActor{
		encoder: encoding.NewProtoEncoder(),
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
	message := fakeMessage{}
	ca := fake.NewAuthority(1, fake.NewSigner)
	rpc := fake.NewRPC()

	actor := flatActor{
		encoder: encoding.NewProtoEncoder(),
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
	message := fakeMessage{}
	ca := fake.NewAuthority(1, fake.NewSigner)

	rpc := fake.NewRPC()
	actor := flatActor{
		encoder: encoding.NewProtoEncoder(),
		signer:  ca.GetSigner(0),
		rpc:     rpc,
		reactor: fakeReactor{},
	}

	rpc.Msgs <- &empty.Empty{}
	close(rpc.Msgs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := actor.Sign(ctx, message, ca)
	require.EqualError(t, err,
		"couldn't process response: response type is invalid: *empty.Empty")

	actor.signer = fake.NewBadSigner()
	_, err = actor.processResponse(&SignatureResponse{}, fake.Signature{})
	require.EqualError(t, err, "couldn't aggregate: fake error")

	actor.signer = fake.NewSignerWithSignatureFactory(fake.NewBadSignatureFactory())
	_, err = actor.processResponse(&SignatureResponse{}, nil)
	require.EqualError(t, err, "couldn't decode signature: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

func makePackedSignature(t *testing.T, signer crypto.Signer, message []byte) *any.Any {
	sig, err := signer.Sign(message)
	require.NoError(t, err)

	packed, err := sig.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)

	packedAny, err := ptypes.MarshalAny(packed)
	require.NoError(t, err)

	return packedAny
}

type fakeMessage struct {
	err error
}

func (m fakeMessage) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, m.err
}
