package types

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

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

func TestForwardLink_Hash(t *testing.T) {
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

func TestChainFactory_Verify(t *testing.T) {
	factory := ChainFactory{
		verifierFactory: fake.VerifierFactory{},
		viewchange:      fakeViewChange{},
	}

	chain := Chain{links: []ForwardLink{{prepare: fake.Signature{}}}}

	// Empty chain is verified all the time.
	err := factory.verify(chain)
	require.NoError(t, err)

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
}

func (vc fakeViewChange) GetAuthority(uint64) (viewchange.Authority, error) {
	return roster.FromAuthority(fake.NewAuthority(3, fake.NewSigner)), nil
}
