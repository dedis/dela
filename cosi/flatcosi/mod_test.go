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
	"go.dedis.ch/fabric/mino"
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
	flat := NewFlat(fakeMino{}, bls.NewSigner())

	a, err := flat.Listen(fakeHasher{})
	require.NoError(t, err)
	actor := a.(flatActor)
	require.NotNil(t, actor.signer)
	require.NotNil(t, actor.rpc)

	flat.mino = fakeMino{err: xerrors.New("oops")}
	_, err = flat.Listen(fakeHasher{})
	require.EqualError(t, err, "couldn't make the rpc: oops")
}

func TestActor_Sign(t *testing.T) {
	message := fakeMessage{}
	ca := fake.NewAuthority(1, fake.NewSigner)

	msgs := make(chan proto.Message, 2)
	actor := flatActor{
		encoder: encoding.NewProtoEncoder(),
		signer:  fake.NewSigner(),
		rpc:     fakeRPC{msgs: msgs},
	}

	sigAny := makePackedSignature(t, actor.signer, message.GetHash())

	msgs <- &SignatureResponse{Signature: sigAny}
	msgs <- &SignatureResponse{Signature: sigAny}
	close(msgs)

	ctx := context.Background()

	sig, err := actor.Sign(ctx, message, ca)
	require.NoError(t, err)
	require.NotNil(t, sig)

	actor.encoder = badPackAnyEncoder{}
	_, err = actor.Sign(ctx, message, nil)
	require.EqualError(t, err, "couldn't pack message: oops")

	actor.encoder = encoding.NewProtoEncoder()
	actor.signer = fake.NewSignerWithVerifierFactory(fake.NewBadVerifierFactory())
	_, err = actor.Sign(ctx, message, ca)
	require.EqualError(t, err, "couldn't make verifier: fake error")
}

func TestActor_SignWrongSignature(t *testing.T) {
	message := fakeMessage{}
	ca := fake.NewAuthority(1, bls.NewSigner)

	msgs := make(chan proto.Message, 1)
	actor := flatActor{
		encoder: encoding.NewProtoEncoder(),
		signer:  ca.GetSigner(0),
		rpc:     fakeRPC{msgs: msgs},
	}

	sigAny := makePackedSignature(t, ca.GetSigner(0), []byte{0xef})

	msgs <- &SignatureResponse{Signature: sigAny}
	close(msgs)

	ctx := context.Background()

	_, err := actor.Sign(ctx, message, ca)
	require.EqualError(t, err, "couldn't verify the aggregation: bls: invalid signature")
}

func TestActor_RPCError_Sign(t *testing.T) {
	buffer := new(bytes.Buffer)
	message := fakeMessage{}
	ca := fake.NewAuthority(1, fake.NewSigner)

	msgs := make(chan proto.Message)
	errs := make(chan error)
	actor := flatActor{
		encoder: encoding.NewProtoEncoder(),
		logger:  zerolog.New(buffer),
		signer:  ca.GetSigner(0),
		rpc:     fakeRPC{msgs: msgs, errs: errs},
	}

	go func() {
		errs <- xerrors.New("oops")
		close(msgs)
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

	msgs := make(chan proto.Message)
	errs := make(chan error)
	actor := flatActor{
		encoder: encoding.NewProtoEncoder(),
		logger:  zerolog.New(buffer),
		signer:  ca.GetSigner(0),
		rpc:     fakeRPC{msgs: msgs, errs: errs},
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

	msgs := make(chan proto.Message, 1)
	actor := flatActor{
		encoder: encoding.NewProtoEncoder(),
		logger:  zerolog.New(buffer),
		signer:  ca.GetSigner(0),
		rpc:     fakeRPC{msgs: msgs},
	}

	msgs <- &empty.Empty{}
	close(msgs)

	ctx := context.Background()

	_, err := actor.Sign(ctx, message, ca)
	require.EqualError(t, err, "signature is nil")
	require.Contains(t, buffer.String(), "error when processing response")

	actor.signer = fakeSigner{err: xerrors.New("oops")}
	_, err = actor.processResponse(&SignatureResponse{}, fakeSignature{})
	require.EqualError(t, err, "couldn't aggregate: oops")

	actor.signer = fakeSigner{errSignatureFactory: xerrors.New("oops")}
	_, err = actor.processResponse(&SignatureResponse{}, nil)
	require.EqualError(t, err, "couldn't decode signature: oops")
}

//------------------
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

type badPackAnyEncoder struct {
	encoding.ProtoEncoder
}

func (e badPackAnyEncoder) PackAny(encoding.Packable) (*any.Any, error) {
	return nil, xerrors.New("oops")
}

type badUnmarshalDynEncoder struct {
	encoding.ProtoEncoder
}

func (e badUnmarshalDynEncoder) UnmarshalDynamicAny(*any.Any) (proto.Message, error) {
	return nil, xerrors.New("oops")
}

type badVerifierFactory struct {
	crypto.VerifierFactory
}

func (f badVerifierFactory) FromIterator(crypto.PublicKeyIterator) (crypto.Verifier, error) {
	return nil, xerrors.New("oops")
}

type fakeSignatureFactory struct {
	crypto.SignatureFactory
	err error
}

func (f fakeSignatureFactory) FromProto(proto.Message) (crypto.Signature, error) {
	return nil, f.err
}

type fakeSigner struct {
	crypto.AggregateSigner
	err                 error
	errSignatureFactory error
	errSignature        error
}

func (s fakeSigner) GetVerifierFactory() crypto.VerifierFactory {
	return badVerifierFactory{}
}

func (s fakeSigner) GetSignatureFactory() crypto.SignatureFactory {
	return fakeSignatureFactory{err: s.errSignatureFactory}
}

func (s fakeSigner) Sign([]byte) (crypto.Signature, error) {
	return fakeSignature{err: s.errSignature}, s.err
}

func (s fakeSigner) Aggregate(...crypto.Signature) (crypto.Signature, error) {
	return nil, s.err
}

type fakeRPC struct {
	mino.RPC
	msgs chan proto.Message
	errs chan error
}

func (rpc fakeRPC) Call(ctx context.Context, msg proto.Message,
	players mino.Players) (<-chan proto.Message, <-chan error) {

	select {
	case <-ctx.Done():
		go func() {
			rpc.errs <- ctx.Err()
			close(rpc.msgs)
		}()
	default:
	}

	return rpc.msgs, rpc.errs
}

type fakeMino struct {
	mino.Mino
	err error
}

func (m fakeMino) MakeRPC(name string, h mino.Handler) (mino.RPC, error) {
	return fakeRPC{}, m.err
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

type fakeSignature struct {
	crypto.Signature
	err error
}

func (s fakeSignature) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, s.err
}
