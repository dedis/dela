package types

import (
	"bytes"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	RegisterForwardLinkFormat(fake.GoodFormat, fake.Format{Msg: ForwardLink{}})
	RegisterForwardLinkFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterChainFormat(fake.GoodFormat, fake.Format{
		Msg: NewChain(ForwardLink{prepare: fake.Signature{}, commit: fake.Signature{}}),
	})
	RegisterChainFormat(serde.Format("BAD_TYPE"), fake.Format{Msg: fake.Message{}})
	RegisterChainFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestForwardLink_Getters(t *testing.T) {
	f := func(from, to []byte) bool {
		fl, err := NewForwardLink(from, to, WithHashFactory(crypto.NewSha256Factory()))
		require.NoError(t, err)
		require.Len(t, fl.GetFingerprint(), 32)

		return bytes.Equal(from, fl.GetFrom()) && bytes.Equal(to, fl.GetTo())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestForwardLink_GetPrepareSignature(t *testing.T) {
	fl, err := NewForwardLink(nil, nil, WithPrepare(fake.Signature{}))
	require.NoError(t, err)

	require.Equal(t, fake.Signature{}, fl.GetPrepareSignature())
}

func TestForwardLink_GetCommitSignature(t *testing.T) {
	fl, err := NewForwardLink(nil, nil, WithCommit(fake.Signature{}))
	require.NoError(t, err)

	require.Equal(t, fake.Signature{}, fl.GetCommitSignature())
}

func TestForwardLink_GetChangeSet(t *testing.T) {
	fl, err := NewForwardLink(nil, nil, WithChangeSet(roster.ChangeSet{}))
	require.NoError(t, err)

	require.Equal(t, roster.ChangeSet{}, fl.GetChangeSet())
}

func TestForwardLink_Verify(t *testing.T) {
	fl := ForwardLink{
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

func TestForwardLink_Serialize(t *testing.T) {
	fl := ForwardLink{}

	data, err := fl.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = fl.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode link: fake error")
}

func TestForwardLink_Fingerprint(t *testing.T) {
	fl := ForwardLink{
		from: []byte{0xaa},
		to:   []byte{0xbb},
	}

	out := new(bytes.Buffer)
	err := fl.Fingerprint(out)
	require.NoError(t, err)
	require.Equal(t, "\xaa\xbb", out.String())

	err = fl.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write 'from': fake error")

	err = fl.Fingerprint(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err, "couldn't write 'to': fake error")
}

func TestChain_Len(t *testing.T) {
	chain := NewChain(ForwardLink{}, ForwardLink{})

	require.Equal(t, 2, chain.Len())
}

func TestChain_GetLinks(t *testing.T) {
	chain := NewChain(ForwardLink{}, ForwardLink{})

	require.Len(t, chain.GetLinks(), 2)
}

func TestChain_GetTo(t *testing.T) {
	chain := Chain{}
	require.Nil(t, chain.GetTo())

	hash := Digest{1, 2, 3}
	chain = Chain{
		links: []ForwardLink{
			{prepare: fake.Signature{}, commit: fake.Signature{}},
			{to: hash, prepare: fake.Signature{}, commit: fake.Signature{}},
		},
	}

	require.Equal(t, hash, chain.GetTo())
}

func TestChain_Serialize(t *testing.T) {
	chain := Chain{}

	data, err := chain.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = chain.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode chain: fake error")
}

func TestChainFactory_Deserialize(t *testing.T) {
	factory := NewChainFactory(
		fake.SignatureFactory{},
		nil,
		fakeViewChange{},
		fake.VerifierFactory{})

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.IsType(t, Chain{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode chain: fake error")

	_, err = factory.Deserialize(fake.NewContextWithFormat(serde.Format("BAD_TYPE")), nil)
	require.EqualError(t, err, "invalid message of type 'fake.Message'")

	factory.viewchange = fakeViewChange{err: xerrors.New("oops")}
	_, err = factory.Deserialize(fake.NewContext(), nil)
	require.EqualError(t, err, "couldn't verify the chain: couldn't get authority: oops")
}

func TestChainFactory_Verify(t *testing.T) {
	factory := ChainFactory{
		verifierFactory: fake.VerifierFactory{},
		viewchange:      fakeViewChange{},
	}

	chain := Chain{links: []ForwardLink{{prepare: fake.Signature{}}}}

	// Empty chain is verified all the time.
	err := factory.verify(chain)
	require.NoError(t, err)

	require.NoError(t, factory.verify(NewChain()))

	factory.verifierFactory = fake.NewVerifierFactory(fake.NewBadVerifier())
	err = factory.verify(chain)
	require.EqualError(t, err,
		"couldn't verify link 0: couldn't verify prepare signature: fake error")

	chain.links = []ForwardLink{
		{to: []byte{0xc}, prepare: fake.Signature{}},
		{from: []byte{0xb}, prepare: fake.Signature{}},
	}
	factory.verifierFactory = fake.VerifierFactory{}
	err = factory.verify(chain)
	require.EqualError(t, err, "mismatch forward link '0c' != '0b'")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeViewChange struct {
	viewchange.ViewChange

	err error
}

func (vc fakeViewChange) GetAuthority(uint64) (viewchange.Authority, error) {
	return roster.FromAuthority(fake.NewAuthority(3, fake.NewSigner)), vc.err
}
