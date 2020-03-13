package flatcosi

import (
	"bytes"
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
	signer := bls.NewSigner()
	flat := NewFlat(nil, signer)
	require.NotNil(t, flat.GetPublicKeyFactory())
}

func TestFlat_GetSignatureFactory(t *testing.T) {
	signer := bls.NewSigner()
	flat := NewFlat(nil, signer)
	require.NotNil(t, flat.GetSignatureFactory())
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

func TestFlat_GetVerifier(t *testing.T) {
	signer := bls.NewSigner()
	flat := NewFlat(nil, signer)

	verifier, err := flat.GetVerifier(fakeCollectiveAuthority{})
	require.NoError(t, err)

	verifier, err = flat.GetVerifier(nil)
	require.Error(t, err)
	require.Nil(t, verifier)

	flat.signer = fakeSigner{}
	verifier, err = flat.GetVerifier(fakeCollectiveAuthority{})
	require.EqualError(t, err, "couldn't create verifier: oops")
}

type fakeRPC struct {
	mino.RPC
	msgs chan proto.Message
	errs chan error
}

func (rpc fakeRPC) Call(msg proto.Message, players mino.Players) (<-chan proto.Message, <-chan error) {
	return rpc.msgs, rpc.errs
}

type fakeMino struct {
	mino.Mino
	err error
}

func (m fakeMino) MakeRPC(name string, h mino.Handler) (mino.RPC, error) {
	return fakeRPC{}, m.err
}

func TestFlat_Listen(t *testing.T) {
	flat := NewFlat(fakeMino{}, bls.NewSigner())

	a, err := flat.Listen(fakeHasher{})
	require.NoError(t, err)
	actor := a.(flatActor)
	require.NotNil(t, actor.signer)
	require.NotNil(t, actor.rpc)

	flat.mino = fakeMino{err: xerrors.New("oops")}
	a, err = flat.Listen(fakeHasher{})
	require.EqualError(t, err, "couldn't make the rpc: oops")
}

type fakeMessage struct {
	cosi.Message
	err error
}

func (m fakeMessage) GetHash() []byte {
	return []byte{0xab}
}

func (m fakeMessage) Pack() (proto.Message, error) {
	return &empty.Empty{}, m.err
}

type fakeIterator struct {
	crypto.PublicKeyIterator
	count  int
	pubkey crypto.PublicKey
}

func (i *fakeIterator) HasNext() bool {
	return i.count > 0 && i.pubkey != nil
}

func (i *fakeIterator) GetNext() crypto.PublicKey {
	i.count--
	return i.pubkey
}

type fakeCollectiveAuthority struct {
	cosi.CollectiveAuthority
	pubkey crypto.PublicKey
}

func (ca fakeCollectiveAuthority) PublicKeyIterator() crypto.PublicKeyIterator {
	return &fakeIterator{count: 2, pubkey: ca.pubkey}
}

type fakeProtoEncoder struct {
	encoding.ProtoEncoder
	errUnmarshal error
}

func (e fakeProtoEncoder) MarshalAny(pb proto.Message) (*any.Any, error) {
	return nil, xerrors.New("oops")
}

func (e fakeProtoEncoder) UnmarshalAny(*any.Any, proto.Message) error {
	return e.errUnmarshal
}

func TestActor_Sign(t *testing.T) {
	defer func() { protoenc = encoding.NewProtoEncoder() }()

	message := fakeMessage{}

	msgs := make(chan proto.Message, 2)
	actor := flatActor{
		signer: bls.NewSigner(),
		rpc:    fakeRPC{msgs: msgs},
	}

	sigAny := makePackedSignature(t, actor.signer, message.GetHash())
	ca := fakeCollectiveAuthority{pubkey: actor.signer.GetPublicKey()}

	msgs <- &SignatureResponse{Signature: sigAny}
	msgs <- &SignatureResponse{Signature: sigAny}
	close(msgs)

	sig, err := actor.Sign(message, ca)
	require.NoError(t, err)
	require.NotNil(t, sig)

	_, err = actor.Sign(fakeMessage{err: xerrors.New("oops")}, nil)
	require.EqualError(t, err, "couldn't encode message: oops")

	protoenc = fakeProtoEncoder{}
	_, err = actor.Sign(message, nil)
	require.EqualError(t, err, "couldn't encode any *empty.Empty: oops")

	protoenc = encoding.NewProtoEncoder()
	actor.signer = fakeSigner{}
	_, err = actor.Sign(message, ca)
	require.EqualError(t, err, "couldn't make verifier: oops")
}

func TestActor_SignWrongSignature(t *testing.T) {
	message := fakeMessage{}

	msgs := make(chan proto.Message, 1)
	actor := flatActor{
		signer: bls.NewSigner(),
		rpc:    fakeRPC{msgs: msgs},
	}

	sigAny := makePackedSignature(t, actor.signer, message.GetHash())

	msgs <- &SignatureResponse{Signature: sigAny}
	close(msgs)

	_, err := actor.Sign(message, fakeCollectiveAuthority{})
	require.EqualError(t, err, "couldn't verify the aggregation: bls: invalid signature")
}

func TestActor_SignRPCError(t *testing.T) {
	buffer := new(bytes.Buffer)
	message := fakeMessage{}

	msgs := make(chan proto.Message)
	errs := make(chan error)
	actor := flatActor{
		logger: zerolog.New(buffer),
		signer: bls.NewSigner(),
		rpc:    fakeRPC{msgs: msgs, errs: errs},
	}

	go func() {
		errs <- xerrors.New("oops")
		close(msgs)
	}()

	sig, err := actor.Sign(message, fakeCollectiveAuthority{})
	require.EqualError(t, err, "signature is nil")
	require.Nil(t, sig)
	require.Contains(t, buffer.String(), "error during collective signing")
}

type fakeSignature struct {
	crypto.Signature
	err error
}

func (s fakeSignature) Pack() (proto.Message, error) {
	return &empty.Empty{}, s.err
}

func TestActor_SignProcessError(t *testing.T) {
	buffer := new(bytes.Buffer)
	message := fakeMessage{}

	msgs := make(chan proto.Message, 1)
	actor := flatActor{
		logger: zerolog.New(buffer),
		signer: bls.NewSigner(),
		rpc:    fakeRPC{msgs: msgs},
	}

	msgs <- &empty.Empty{}
	close(msgs)

	_, err := actor.Sign(message, fakeCollectiveAuthority{})
	require.EqualError(t, err, "signature is nil")
	require.Contains(t, buffer.String(), "error when processing response")

	actor.signer = fakeSigner{err: xerrors.New("oops")}
	_, err = actor.processResponse(&SignatureResponse{}, fakeSignature{})
	require.EqualError(t, err, "couldn't aggregate: oops")

	actor.signer = fakeSigner{errSignatureFactory: xerrors.New("oops")}
	_, err = actor.processResponse(&SignatureResponse{}, nil)
	require.EqualError(t, err, "couldn't decode signature: oops")
}

func makePackedSignature(t *testing.T, signer crypto.Signer, message []byte) *any.Any {
	sig, err := signer.Sign(message)
	require.NoError(t, err)

	packed, err := sig.Pack()
	require.NoError(t, err)

	packedAny, err := ptypes.MarshalAny(packed)
	require.NoError(t, err)

	return packedAny
}
