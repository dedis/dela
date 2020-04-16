package flatcosi

import (
	"bytes"
	"context"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/crypto/bls"
	"go.dedis.ch/fabric/encoding"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/internal/testing/fake"
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

func TestFlat_GetPublicKeyFactory(t *testing.T) {
	signer := fake.NewSigner()
	flat := NewFlat(nil, signer)
	require.NotNil(t, flat.GetPublicKeyFactory())
}

func TestFlat_GetSignatureFactory(t *testing.T) {
	signer := fake.NewSigner()
	flat := NewFlat(nil, signer)
	require.NotNil(t, flat.GetSignatureFactory())
}

func TestFlat_GetVerifier(t *testing.T) {
	ca := fake.NewAuthority(2, fake.NewSigner)
	signer := fake.NewSigner()
	flat := NewFlat(nil, signer)

	_, err := flat.GetVerifier(ca)
	require.NoError(t, err)

	verifier, err := flat.GetVerifier(nil)
	require.Error(t, err)
	require.Nil(t, verifier)

	flat.signer = fake.NewSignerWithVerifierFactory(fake.NewBadVerifierFactory())
	_, err = flat.GetVerifier(ca)
	require.EqualError(t, err, "couldn't create verifier: fake error")
}

func TestFlat_Listen(t *testing.T) {
	flat := NewFlat(fake.Mino{}, bls.NewSigner())

	a, err := flat.Listen(fakeHasher{})
	require.NoError(t, err)
	actor := a.(flatActor)
	require.NotNil(t, actor.signer)
	require.NotNil(t, actor.rpc)

	flat.mino = fake.NewBadMino()
	_, err = flat.Listen(fakeHasher{})
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
	}

	sigAny := makePackedSignature(t, actor.signer, message.GetHash())

	rpc.Msgs <- &SignatureResponse{Signature: sigAny}
	rpc.Msgs <- &SignatureResponse{Signature: sigAny}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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
	}

	sigAny := makePackedSignature(t, ca.GetSigner(0), []byte{0xef})

	rpc.Msgs <- &SignatureResponse{Signature: sigAny}
	close(rpc.Msgs)

	ctx := context.Background()

	_, err := actor.Sign(ctx, message, ca)
	require.EqualError(t, err, "couldn't verify the aggregation: bls: invalid signature")
}

func TestActor_RPCError_Sign(t *testing.T) {
	buffer := new(bytes.Buffer)
	message := fakeMessage{}
	ca := fake.NewAuthority(1, fake.NewSigner)

	rpc := fake.NewRPC()
	actor := flatActor{
		encoder: encoding.NewProtoEncoder(),
		logger:  zerolog.New(buffer),
		signer:  ca.GetSigner(0),
		rpc:     rpc,
	}

	go func() {
		rpc.Errs <- xerrors.New("oops")
		close(rpc.Msgs)
	}()

	ctx := context.Background()

	sig, err := actor.Sign(ctx, message, ca)
	require.EqualError(t, err, "signature is nil")
	require.Nil(t, sig)
	require.Contains(t, buffer.String(), "error during collective signing")
}

func TestActor_Context_Sign(t *testing.T) {
	buffer := new(bytes.Buffer)
	message := fakeMessage{}
	ca := fake.NewAuthority(1, fake.NewSigner)

	actor := flatActor{
		encoder: encoding.NewProtoEncoder(),
		logger:  zerolog.New(buffer),
		signer:  ca.GetSigner(0),
		rpc:     fake.NewRPC(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sig, err := actor.Sign(ctx, message, ca)
	require.EqualError(t, err, "signature is nil")
	require.Nil(t, sig)
	require.Contains(t, buffer.String(), "error during collective signing")
}

func TestActor_SignProcessError(t *testing.T) {
	buffer := new(bytes.Buffer)
	message := fakeMessage{}
	ca := fake.NewAuthority(1, fake.NewSigner)

	rpc := fake.NewRPC()
	actor := flatActor{
		encoder: encoding.NewProtoEncoder(),
		logger:  zerolog.New(buffer),
		signer:  ca.GetSigner(0),
		rpc:     rpc,
	}

	rpc.Msgs <- &empty.Empty{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := actor.Sign(ctx, message, ca)
	require.EqualError(t, err, "signature is nil")
	require.Contains(t, buffer.String(), "error when processing response")

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
	cosi.Message
	err error
}

func (m fakeMessage) GetHash() []byte {
	return []byte{0xab}
}

func (m fakeMessage) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, m.err
}
