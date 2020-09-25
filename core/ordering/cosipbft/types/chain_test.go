package types

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func init() {
	RegisterLinkFormat(fake.GoodFormat, fake.Format{Msg: blockLink{}})
	RegisterLinkFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterLinkFormat(serde.Format("badtype"), fake.Format{Msg: fake.Message{}})
	RegisterChainFormat(fake.GoodFormat, fake.Format{Msg: chain{}})
	RegisterChainFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterChainFormat(serde.Format("badtype"), fake.Format{Msg: fake.Message{}})
}

func TestForwardLink_New(t *testing.T) {
	link, err := NewForwardLink(Digest{1}, Digest{2})
	require.NoError(t, err)
	require.Equal(t, Digest{1}, link.GetFrom())

	opts := []LinkOption{
		WithSignatures(fake.Signature{}, fake.Signature{}),
		WithChangeSet(authority.NewChangeSet()),
	}

	link, err = NewForwardLink(Digest{1}, Digest{2}, opts...)
	require.NoError(t, err)
	require.Equal(t, fake.Signature{}, link.GetPrepareSignature())
	require.Equal(t, fake.Signature{}, link.GetCommitSignature())

	opts = []LinkOption{
		WithLinkHashFactory(fake.NewHashFactory(fake.NewBadHash())),
	}

	_, err = NewForwardLink(Digest{1}, Digest{2}, opts...)
	require.EqualError(t, err, fake.Err("failed to fingerprint: couldn't write from"))
}

func TestForwardLink_GetHash(t *testing.T) {
	link := forwardLink{digest: Digest{1}}

	require.Equal(t, Digest{1}, link.GetHash())
}

func TestForwardLink_GetFrom(t *testing.T) {
	link := forwardLink{from: Digest{2}}

	require.Equal(t, Digest{2}, link.GetFrom())
}

func TestForwardLink_GetTo(t *testing.T) {
	link := forwardLink{to: Digest{3}}

	require.Equal(t, Digest{3}, link.GetTo())
}

func TestForwardLink_GetPrepareSignature(t *testing.T) {
	link := forwardLink{prepareSig: fake.Signature{}}

	require.NotNil(t, link.GetPrepareSignature())
	require.Nil(t, link.GetCommitSignature())
}

func TestForwardLink_GetCommitSignature(t *testing.T) {
	link := forwardLink{commitSig: fake.Signature{}}

	require.NotNil(t, link.GetCommitSignature())
	require.Nil(t, link.GetPrepareSignature())
}

func TestForwardLink_GetChangeSet(t *testing.T) {
	link := forwardLink{
		changeset: authority.NewChangeSet(),
	}

	require.Equal(t, authority.NewChangeSet(), link.GetChangeSet())
}

func TestForwardLink_Serialize(t *testing.T) {
	link := forwardLink{}

	data, err := link.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = link.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("encoding link failed"))
}

func TestForwardLink_Fingerprint(t *testing.T) {
	link, err := NewForwardLink(Digest{1}, Digest{2})
	require.NoError(t, err)

	buffer := new(bytes.Buffer)

	err = link.Fingerprint(buffer)
	require.NoError(t, err)
	require.Regexp(t, "^\x01\x00{31}\x02\x00{31}$", buffer.String())

	err = link.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, fake.Err("couldn't write from"))

	err = link.Fingerprint(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err, fake.Err("couldn't write to"))
}

func TestBlockLink_New(t *testing.T) {
	link, err := NewBlockLink(Digest{}, Block{})
	require.NoError(t, err)
	require.Equal(t, Digest{}, link.GetFrom())

	opt := WithLinkHashFactory(fake.NewHashFactory(fake.NewBadHash()))

	_, err = NewBlockLink(Digest{}, Block{}, opt)
	require.EqualError(t, err,
		fake.Err("creating forward link: failed to fingerprint: couldn't write from"))
}

func TestBlockLink_GetBlock(t *testing.T) {
	link := blockLink{
		block: Block{index: 1},
	}

	require.Equal(t, uint64(1), link.GetBlock().GetIndex())
}

func TestBlockLink_Reduce(t *testing.T) {
	link := blockLink{
		forwardLink: forwardLink{
			from:       Digest{1},
			to:         Digest{2},
			prepareSig: fake.Signature{},
			commitSig:  fake.Signature{},
		},
	}

	require.Equal(t, link.forwardLink, link.Reduce())
}

func TestBlockLink_Serialize(t *testing.T) {
	link := blockLink{}

	data, err := link.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = link.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("encoding failed"))
}

func TestLinkFac_LinkOf(t *testing.T) {
	csFac := authority.NewChangeSetFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	fac := NewLinkFactory(BlockFactory{}, fake.SignatureFactory{}, csFac)

	msg, err := fac.LinkOf(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, blockLink{}, msg)

	_, err = fac.LinkOf(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("decoding link failed"))

	_, err = fac.LinkOf(fake.NewContextWithFormat(serde.Format("badtype")), nil)
	require.EqualError(t, err, "invalid forward link 'fake.Message'")
}

func TestLinkFac_BlockLinkOf(t *testing.T) {
	csFac := authority.NewChangeSetFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	fac := NewLinkFactory(BlockFactory{}, fake.SignatureFactory{}, csFac)

	msg, err := fac.BlockLinkOf(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, blockLink{}, msg)

	_, err = fac.BlockLinkOf(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("decoding link failed"))

	_, err = fac.BlockLinkOf(fake.NewContextWithFormat(serde.Format("badtype")), nil)
	require.EqualError(t, err, "invalid block link 'fake.Message'")
}

func TestChain_GetLinks(t *testing.T) {
	chain := NewChain(blockLink{}, []Link{forwardLink{}, forwardLink{}})

	require.Len(t, chain.GetLinks(), 3)
}

func TestChain_GetBlock(t *testing.T) {
	chain := NewChain(blockLink{block: Block{index: 2}}, nil)

	require.Equal(t, uint64(2), chain.GetBlock().GetIndex())
}

func TestChain_Verify(t *testing.T) {
	ro := authority.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	genesis, err := NewGenesis(ro)
	require.NoError(t, err)

	c := NewChain(makeLink(t, genesis.digest), nil)

	err = c.Verify(genesis, fake.VerifierFactory{})
	require.NoError(t, err)

	err = c.Verify(genesis, fake.VerifierFactory{})
	require.NoError(t, err)

	c = NewChain(makeLink(t, Digest{}), nil)
	err = c.Verify(genesis, fake.VerifierFactory{})
	require.EqualError(t, err, fmt.Sprintf("mismatch from: '00000000' != '%v'", genesis.GetHash()))

	c = NewChain(makeLink(t, genesis.digest), nil)
	err = c.Verify(genesis, fake.NewBadVerifierFactory())
	require.EqualError(t, err, fake.Err("verifier factory failed"))

	err = c.Verify(genesis, fake.NewVerifierFactory(fake.NewBadVerifier()))
	require.EqualError(t, err, fake.Err("invalid prepare signature"))

	link := makeLink(t, genesis.digest).(blockLink)
	link.prepareSig = nil
	c = NewChain(link, nil)
	err = c.Verify(genesis, fake.VerifierFactory{})
	require.EqualError(t, err, "unexpected nil prepare signature in link")

	link.prepareSig = fake.Signature{}
	link.commitSig = nil
	c = NewChain(link, nil)
	err = c.Verify(genesis, fake.VerifierFactory{})
	require.EqualError(t, err, "unexpected nil commit signature in link")

	link.prepareSig = fake.NewBadSignature()
	link.commitSig = fake.Signature{}
	c = NewChain(link, nil)
	err = c.Verify(genesis, fake.VerifierFactory{})
	require.EqualError(t, err, fake.Err("failed to marshal signature"))

	c = NewChain(makeLink(t, genesis.digest), nil)
	err = c.Verify(genesis, fake.NewVerifierFactory(fake.NewBadVerifierWithDelay(1)))
	require.EqualError(t, err, fake.Err("invalid commit signature"))
}

func TestChain_Serialize(t *testing.T) {
	chain := chain{}

	data, err := chain.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = chain.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("encoding chain failed"))
}

func TestChainFactory_Deserialize(t *testing.T) {
	fac := NewChainFactory(linkFac{})

	msg, err := fac.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, chain{}, msg)

	_, err = fac.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("decoding chain failed"))

	_, err = fac.Deserialize(fake.NewContextWithFormat(serde.Format("badtype")), nil)
	require.EqualError(t, err, "invalid chain 'fake.Message'")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeLink(t *testing.T, from Digest) BlockLink {
	link, err := NewForwardLink(from, Digest{}, WithSignatures(fake.Signature{}, fake.Signature{}))
	require.NoError(t, err)

	return blockLink{forwardLink: link.(forwardLink)}
}
