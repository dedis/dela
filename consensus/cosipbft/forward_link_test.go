package cosipbft

import (
	"crypto/sha256"
	"hash"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"golang.org/x/xerrors"
)

func TestForwardLink_Verify(t *testing.T) {
	fl := forwardLink{
		hash:    []byte{0xaa},
		prepare: fakeSignature{},
	}

	verifier := &fakeVerifier{}

	err := fl.Verify(verifier)
	require.NoError(t, err)
	require.Len(t, verifier.calls, 2)
	require.Equal(t, []byte{0xaa}, verifier.calls[0]["message"])
	require.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, verifier.calls[1]["message"])

	verifier.err = xerrors.New("oops")
	err = fl.Verify(verifier)
	require.EqualError(t, err, "couldn't verify prepare signature: oops")

	verifier.delay = 1
	err = fl.Verify(verifier)
	require.EqualError(t, err, "couldn't verify commit signature: oops")

	verifier.err = nil
	fl.prepare = fakeSignature{err: xerrors.New("oops")}
	err = fl.Verify(verifier)
	require.EqualError(t, err, "couldn't marshal the signature: oops")
}

func TestForwardLink_Pack(t *testing.T) {
	fl := forwardLink{
		from: []byte{0xaa},
		to:   []byte{0xbb},
	}

	pb, err := fl.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	flp, ok := pb.(*ForwardLinkProto)
	require.True(t, ok)
	require.Equal(t, flp.GetFrom(), fl.from)
	require.Equal(t, flp.GetTo(), fl.to)
	require.Nil(t, flp.GetPrepare())
	require.Nil(t, flp.GetCommit())

	fl.prepare = fakeSignature{value: 1}
	pb, err = fl.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	flp = pb.(*ForwardLinkProto)
	checkSignatureValue(t, flp.GetPrepare(), 1)

	fl.commit = fakeSignature{value: 2}
	pb, err = fl.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	flp = pb.(*ForwardLinkProto)
	checkSignatureValue(t, flp.GetCommit(), 2)

	// Test if the prepare signature cannot be packed.
	fl.prepare = fakeSignature{}
	_, err = fl.Pack(badPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack prepare signature: oops")

	// Test if the commit signature cannot be packed.
	fl.prepare = nil
	fl.commit = fakeSignature{}
	_, err = fl.Pack(badPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack commit signature: oops")
}

type fakeHash struct {
	hash.Hash
	delay int
}

func (h *fakeHash) Write(data []byte) (int, error) {
	if h.delay > 0 {
		h.delay--
		return 0, nil
	}
	return 0, xerrors.New("oops")
}

func TestForwardLink_Hash(t *testing.T) {
	h := sha256.New()

	fl := forwardLink{from: []byte{0xaa}, to: []byte{0xbb}}
	digest, err := fl.computeHash(h)
	require.NoError(t, err)
	require.Len(t, digest, h.Size())

	_, err = fl.computeHash(&fakeHash{})
	require.EqualError(t, err, "couldn't write 'from': oops")

	_, err = fl.computeHash(&fakeHash{delay: 1})
	require.EqualError(t, err, "couldn't write 'to': oops")
}

func TestChain_GetLastHash(t *testing.T) {
	chain := forwardLinkChain{}
	require.Nil(t, chain.GetLastHash())

	hash := Digest{1, 2, 3}
	chain = forwardLinkChain{
		links: []forwardLink{{}, {to: hash}},
	}

	last := chain.GetLastHash()
	require.Equal(t, hash, last)
}

func TestChain_Verify(t *testing.T) {
	chain := forwardLinkChain{
		links: []forwardLink{
			{from: []byte{0xaa}, to: []byte{0xbb}, prepare: fakeSignature{}, commit: fakeSignature{}},
			{from: []byte{0xbb}, to: []byte{0xcc}, prepare: fakeSignature{}, commit: fakeSignature{}},
		},
	}

	verifier := &fakeVerifier{}
	err := chain.Verify(verifier)
	require.NoError(t, err)
	require.Len(t, verifier.calls, 4)

	err = chain.Verify(&fakeVerifier{err: xerrors.New("oops")})
	require.EqualError(t, xerrors.Unwrap(err), "couldn't verify prepare signature: oops")

	chain.links[0].to = []byte{0xff}
	err = chain.Verify(&fakeVerifier{})
	require.EqualError(t, err, "mismatch forward link 'ff' != 'bb'")

	chain.links = nil
	err = chain.Verify(&fakeVerifier{})
	require.EqualError(t, err, "chain is empty")
}

func TestChain_Pack(t *testing.T) {
	chain := forwardLinkChain{
		links: []forwardLink{
			{},
			{},
		},
	}

	pb, err := chain.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*ChainProto)(nil), pb)
	require.Len(t, pb.(*ChainProto).GetLinks(), 2)

	chain.links[0].prepare = fakeSignature{err: xerrors.New("oops")}
	_, err = chain.Pack(badPackEncoder{})
	require.EqualError(t, err, "couldn't pack forward link: oops")
}

type fakeSignatureFactory struct {
	crypto.SignatureFactory
	err    error
	errSig error
}

func (f fakeSignatureFactory) FromProto(pb proto.Message) (crypto.Signature, error) {
	return fakeSignature{err: f.errSig}, f.err
}

func TestChainFactory_FromProto(t *testing.T) {
	chainpb := &ChainProto{
		Links: []*ForwardLinkProto{
			{},
			{},
		},
	}

	factory := newChainFactory(fakeSignatureFactory{})
	chain, err := factory.FromProto(chainpb)
	require.NoError(t, err)
	require.NotNil(t, chain)

	chainany, err := ptypes.MarshalAny(chainpb)
	require.NoError(t, err)

	chain, err = factory.FromProto(chainany)
	require.NoError(t, err)
	require.NotNil(t, chain)

	_, err = factory.FromProto(&empty.Empty{})
	require.EqualError(t, err, "message type not supported: *empty.Empty")

	chainpb.Links[0].Prepare = &any.Any{}
	factory = newChainFactory(fakeSignatureFactory{err: xerrors.New("oops")})
	_, err = factory.FromProto(chainpb)
	require.EqualError(t, err, "couldn't decode prepare signature: oops")

	factory.encoder = badUnmarshalAnyEncoder{}
	_, err = factory.FromProto(chainany)
	require.EqualError(t, err, "couldn't unmarshal message: oops")
}

type badHash struct {
	hash.Hash
}

func (h badHash) Write([]byte) (int, error) {
	return 0, xerrors.New("oops")
}

type badHashFactory struct{}

func (f badHashFactory) New() hash.Hash {
	return badHash{}
}

func TestChainFactory_DecodeForwardLink(t *testing.T) {
	factory := chainFactory{
		encoder:          encoding.NewProtoEncoder(),
		signatureFactory: fakeSignatureFactory{},
		hashFactory:      sha256Factory{},
	}

	forwardLink := &ForwardLinkProto{}
	flany, err := ptypes.MarshalAny(forwardLink)
	require.NoError(t, err)

	chain, err := factory.decodeForwardLink(flany)
	require.NoError(t, err)
	require.NotNil(t, chain)

	_, err = factory.decodeForwardLink(&empty.Empty{})
	require.EqualError(t, err, "unknown message type: *empty.Empty")

	forwardLink.Prepare = &any.Any{}
	factory.signatureFactory = fakeSignatureFactory{err: xerrors.New("oops")}
	_, err = factory.decodeForwardLink(forwardLink)
	require.EqualError(t, err, "couldn't decode prepare signature: oops")

	forwardLink.Prepare = nil
	forwardLink.Commit = &any.Any{}
	_, err = factory.decodeForwardLink(forwardLink)
	require.EqualError(t, err, "couldn't decode commit signature: oops")

	forwardLink.Commit = nil
	factory.hashFactory = badHashFactory{}
	_, err = factory.decodeForwardLink(forwardLink)
	require.EqualError(t, err, "couldn't hash the forward link: couldn't write 'from': oops")

	factory.encoder = badUnmarshalAnyEncoder{}
	_, err = factory.decodeForwardLink(flany)
	require.EqualError(t, err, "couldn't unmarshal forward link: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type badPackAnyEncoder struct {
	encoding.ProtoEncoder
}

func (e badPackAnyEncoder) PackAny(encoding.Packable) (*any.Any, error) {
	return nil, xerrors.New("oops")
}

type badPackEncoder struct {
	encoding.ProtoEncoder
}

func (e badPackEncoder) Pack(encoding.Packable) (proto.Message, error) {
	return nil, xerrors.New("oops")
}

type badUnmarshalAnyEncoder struct {
	encoding.ProtoEncoder
}

func (e badUnmarshalAnyEncoder) UnmarshalAny(*any.Any, proto.Message) error {
	return xerrors.New("oops")
}

type badUnmarshalDynEncoder struct {
	encoding.ProtoEncoder
}

func (e badUnmarshalDynEncoder) UnmarshalDynamicAny(*any.Any) (proto.Message, error) {
	return nil, xerrors.New("oops")
}
