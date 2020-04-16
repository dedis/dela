package cosipbft

import (
	"crypto/sha256"
	"testing"

	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestForwardLink_Verify(t *testing.T) {
	fl := forwardLink{
		hash:    []byte{0xaa},
		prepare: fake.Signature{},
	}

	verifier := &fakeVerifier{}

	err := fl.Verify(verifier)
	require.NoError(t, err)
	require.Len(t, verifier.calls, 2)
	require.Equal(t, []byte{0xaa}, verifier.calls[0]["message"])
	require.Equal(t, []byte{fake.SignatureByte}, verifier.calls[1]["message"])

	verifier.err = xerrors.New("oops")
	err = fl.Verify(verifier)
	require.EqualError(t, err, "couldn't verify prepare signature: oops")

	verifier.delay = 1
	err = fl.Verify(verifier)
	require.EqualError(t, err, "couldn't verify commit signature: oops")

	verifier.err = nil
	fl.prepare = fake.NewBadSignature()
	err = fl.Verify(verifier)
	require.EqualError(t, err, "couldn't marshal the signature: fake error")
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

	fl.prepare = fake.Signature{}
	pb, err = fl.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	flp = pb.(*ForwardLinkProto)
	checkSignatureValue(t, flp.GetPrepare())

	fl.commit = fake.Signature{}
	pb, err = fl.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	flp = pb.(*ForwardLinkProto)
	checkSignatureValue(t, flp.GetCommit())

	// Test if the prepare signature cannot be packed.
	fl.prepare = fake.Signature{}
	_, err = fl.Pack(fake.BadPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack prepare signature: fake error")

	// Test if the commit signature cannot be packed.
	fl.prepare = nil
	fl.commit = fake.Signature{}
	_, err = fl.Pack(fake.BadPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack commit signature: fake error")
}

func TestForwardLink_Hash(t *testing.T) {
	h := sha256.New()

	fl := forwardLink{from: []byte{0xaa}, to: []byte{0xbb}}
	digest, err := fl.computeHash(h)
	require.NoError(t, err)
	require.Len(t, digest, h.Size())

	_, err = fl.computeHash(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write 'from': fake error")

	_, err = fl.computeHash(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err, "couldn't write 'to': fake error")
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
			{from: []byte{0xaa}, to: []byte{0xbb}, prepare: fake.Signature{}, commit: fake.Signature{}},
			{from: []byte{0xbb}, to: []byte{0xcc}, prepare: fake.Signature{}, commit: fake.Signature{}},
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

	chain.links[0].prepare = fake.NewBadSignature()
	_, err = chain.Pack(fake.BadPackEncoder{})
	require.EqualError(t, err, "couldn't pack forward link: fake error")
}

func TestChainFactory_FromProto(t *testing.T) {
	chainpb := &ChainProto{
		Links: []*ForwardLinkProto{
			{},
			{},
		},
	}

	factory := newChainFactory(fake.SignatureFactory{})
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
	factory = newChainFactory(fake.NewBadSignatureFactory())
	_, err = factory.FromProto(chainpb)
	require.EqualError(t, err, "couldn't decode prepare signature: fake error")

	factory.encoder = fake.BadUnmarshalAnyEncoder{}
	_, err = factory.FromProto(chainany)
	require.EqualError(t, err, "couldn't unmarshal message: fake error")
}

func TestChainFactory_DecodeForwardLink(t *testing.T) {
	factory := chainFactory{
		encoder:          encoding.NewProtoEncoder(),
		signatureFactory: fake.SignatureFactory{},
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
	factory.signatureFactory = fake.NewBadSignatureFactory()
	_, err = factory.decodeForwardLink(forwardLink)
	require.EqualError(t, err, "couldn't decode prepare signature: fake error")

	forwardLink.Prepare = nil
	forwardLink.Commit = &any.Any{}
	_, err = factory.decodeForwardLink(forwardLink)
	require.EqualError(t, err, "couldn't decode commit signature: fake error")

	forwardLink.Commit = nil
	factory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = factory.decodeForwardLink(forwardLink)
	require.EqualError(t, err,
		"couldn't hash the forward link: couldn't write 'from': fake error")

	factory.encoder = fake.BadUnmarshalAnyEncoder{}
	_, err = factory.decodeForwardLink(flany)
	require.EqualError(t, err, "couldn't unmarshal forward link: fake error")
}
