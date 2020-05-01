package cosipbft

import (
	"crypto/sha256"
	"testing"

	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/consensus/viewchange"
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
		from:      []byte{0xaa},
		to:        []byte{0xbb},
		changeset: viewchange.ChangeSet{Leader: 5},
	}

	pb, err := fl.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	flp, ok := pb.(*ForwardLinkProto)
	require.True(t, ok)
	require.Equal(t, flp.GetFrom(), fl.from)
	require.Equal(t, flp.GetTo(), fl.to)
	require.Nil(t, flp.GetPrepare())
	require.Nil(t, flp.GetCommit())
	require.Equal(t, &ChangeSet{Leader: 5}, flp.GetChangeSet())

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

	// Test if the changeset cannot be packed.
	fl.changeset.Add = []viewchange.Player{{Address: fake.NewBadAddress()}}
	_, err = fl.Pack(encoding.NewProtoEncoder())
	require.EqualError(t, err,
		"couldn't pack changeset: couldn't pack players: couldn't marshal address: fake error")

	fl.changeset.Add = []viewchange.Player{{Address: fake.NewAddress(0)}}
	_, err = fl.Pack(fake.BadPackAnyEncoder{})
	require.EqualError(t, err,
		"couldn't pack changeset: couldn't pack players: couldn't pack public key: fake error")

	// Test if the prepare signature cannot be packed.
	fl.changeset = viewchange.ChangeSet{}
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

	fl := forwardLink{
		from: []byte{0xaa},
		to:   []byte{0xbb},
	}

	digest, err := fl.computeHash(h, encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.Len(t, digest, h.Size())

	call := &fake.Call{}
	h = &fake.Hash{Call: call}
	fl.changeset.Leader = 5
	fl.changeset.Remove = []uint32{1, 3}
	fl.changeset.Add = []viewchange.Player{{Address: fake.NewAddress(4), PublicKey: fake.PublicKey{}}}
	_, err = fl.computeHash(h, encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.Equal(t, 4, call.Len())
	require.Equal(t, []byte{0xaa}, call.Get(0, 0))
	require.Equal(t, []byte{0xbb}, call.Get(1, 0))
	require.Equal(t, []byte{4, 0, 0, 0, 0xdf}, call.Get(2, 0))
	require.Equal(t, []byte{5, 0, 0, 0, 1, 0, 0, 0, 3, 0, 0, 0}, call.Get(3, 0))

	_, err = fl.computeHash(fake.NewBadHash(), encoding.NewProtoEncoder())
	require.EqualError(t, err, "couldn't write 'from': fake error")

	_, err = fl.computeHash(fake.NewBadHashWithDelay(1), encoding.NewProtoEncoder())
	require.EqualError(t, err, "couldn't write 'to': fake error")

	fl.changeset.Add = []viewchange.Player{{Address: fake.NewBadAddress()}}
	_, err = fl.computeHash(&fake.Hash{}, encoding.NewProtoEncoder())
	require.EqualError(t, err, "couldn't marshal address: fake error")

	fl.changeset.Add = []viewchange.Player{{
		Address:   fake.NewAddress(0),
		PublicKey: fake.NewBadPublicKey(),
	}}
	_, err = fl.computeHash(&fake.Hash{}, encoding.NewProtoEncoder())
	require.EqualError(t, err, "couldn't marshal public key: fake error")

	fl.changeset.Add = []viewchange.Player{{
		Address:   fake.NewAddress(0),
		PublicKey: fake.PublicKey{},
	}}
	_, err = fl.computeHash(fake.NewBadHashWithDelay(2), encoding.NewProtoEncoder())
	require.EqualError(t, err, "couldn't write player: fake error")

	_, err = fl.computeHash(fake.NewBadHashWithDelay(3), encoding.NewProtoEncoder())
	require.EqualError(t, err, "couldn't write integers: fake error")
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

func TestChain_Pack(t *testing.T) {
	chain := forwardLinkChain{
		links: []forwardLink{{}, {}},
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
			makeLinkProto([]byte{0x01}, []byte{0x02}, 1),
			makeLinkProto([]byte{0x02}, []byte{0x03}, 3),
		},
	}

	call := &fake.Call{}
	authority := fake.NewAuthority(3, fake.NewSigner)
	authority.Call = call

	factory := newChainFactory(&fakeCosi{}, fake.Mino{}, authority)
	chain, err := factory.FromProto(chainpb)
	require.NoError(t, err)
	require.NotNil(t, chain)
	require.Equal(t, 2, call.Len())
	require.Equal(t, viewchange.ChangeSet{Leader: 1}, call.Get(0, 1))
	require.Equal(t, viewchange.ChangeSet{Leader: 3}, call.Get(1, 1))

	chainany, err := ptypes.MarshalAny(chainpb)
	require.NoError(t, err)

	chain, err = factory.FromProto(chainany)
	require.NoError(t, err)
	require.NotNil(t, chain)

	_, err = factory.FromProto(&empty.Empty{})
	require.EqualError(t, err, "message type not supported: *empty.Empty")

	chainpb.Links[0].Prepare = &any.Any{}
	factory.signatureFactory = fake.NewBadSignatureFactory()
	_, err = factory.FromProto(chainpb)
	require.EqualError(t, err, "couldn't decode prepare signature: fake error")

	factory.signatureFactory = fake.NewSignatureFactory(fake.Signature{})
	factory.encoder = fake.BadUnmarshalAnyEncoder{}
	_, err = factory.FromProto(chainany)
	require.EqualError(t, err, "couldn't unmarshal message: fake error")

	factory.encoder = encoding.NewProtoEncoder()
	_, err = factory.FromProto(&ChainProto{})
	require.EqualError(t, err, "couldn't verify the chain: chain is empty")

	factory.verifierFactory = fake.NewBadVerifierFactory()
	_, err = factory.FromProto(chainpb)
	require.EqualError(t, err,
		"couldn't verify the chain: couldn't create the verifier: fake error")

	factory.verifierFactory = fake.NewVerifierFactory(fake.NewBadVerifier())
	_, err = factory.FromProto(chainpb)
	require.EqualError(t, err,
		"couldn't verify the chain: couldn't verify link 0: couldn't verify prepare signature: fake error")

	chainpb.Links[0].To = []byte{0x00}
	factory.verifierFactory = fake.VerifierFactory{}
	_, err = factory.FromProto(chainpb)
	require.EqualError(t, err, "couldn't verify the chain: mismatch forward link '00' != '02'")
}

func TestChainFactory_DecodeForwardLink(t *testing.T) {
	factory := unsecureChainFactory{
		encoder:          encoding.NewProtoEncoder(),
		addrFactory:      fake.AddressFactory{},
		pubkeyFactory:    fake.PublicKeyFactory{},
		signatureFactory: fake.SignatureFactory{},
		hashFactory:      sha256Factory{},
	}

	forwardLink := &ForwardLinkProto{
		ChangeSet: &ChangeSet{
			Add: []*Player{{}},
		},
	}
	flany, err := ptypes.MarshalAny(forwardLink)
	require.NoError(t, err)

	chain, err := factory.decodeForwardLink(flany)
	require.NoError(t, err)
	require.NotNil(t, chain)

	_, err = factory.decodeForwardLink(&empty.Empty{})
	require.EqualError(t, err, "unknown message type: *empty.Empty")

	factory.pubkeyFactory = fake.NewBadPublicKeyFactory()
	_, err = factory.decodeForwardLink(forwardLink)
	require.EqualError(t, err,
		"couldn't decode changeset: couldn't decode add: couldn't decode public key: fake error")

	forwardLink.Prepare = &any.Any{}
	factory.pubkeyFactory = fake.PublicKeyFactory{}
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

// -----------------------------------------------------------------------------
// Utility functions

func makeLinkProto(from, to []byte, leader uint32) *ForwardLinkProto {
	return &ForwardLinkProto{
		Prepare: &any.Any{},
		Commit:  &any.Any{},
		From:    from,
		To:      to,
		ChangeSet: &ChangeSet{
			Leader: leader,
		},
	}
}
