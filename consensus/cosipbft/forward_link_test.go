package cosipbft

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus"
	types "go.dedis.ch/dela/consensus/cosipbft/json"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde/json"
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

func TestForwardLink_toJSON(t *testing.T) {
	fl := forwardLink{
		from:    Digest{1},
		to:      Digest{2},
		prepare: fake.Signature{},
		commit:  fake.Signature{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(fl)
	require.NoError(t, err)
	expected := `{"From":"AQ==","To":"Ag==","Prepare":{},"Commit":{},"ChangeSet":null}`
	require.Equal(t, expected, string(data))

	fl.changeset = roster.ChangeSet{}
	_, err = fl.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize changeset: fake error")

	_, err = fl.VisitJSON(fake.NewBadSerializerWithDelay(1))
	require.EqualError(t, err, "couldn't serialize prepare signature: fake error")

	_, err = fl.VisitJSON(fake.NewBadSerializerWithDelay(2))
	require.EqualError(t, err, "couldn't serialize commit signature: fake error")
}

func TestForwardLink_Hash(t *testing.T) {
	h := sha256.New()

	fl := forwardLink{
		from: []byte{0xaa},
		to:   []byte{0xbb},
	}

	digest, err := fl.computeHash(h)
	require.NoError(t, err)
	require.Len(t, digest, h.Size())

	_, err = fl.computeHash(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write 'from': fake error")

	_, err = fl.computeHash(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err, "couldn't write 'to': fake error")
}

func TestChain_GetTo(t *testing.T) {
	chain := forwardLinkChain{}
	require.Nil(t, chain.GetTo())

	hash := Digest{1, 2, 3}
	chain = forwardLinkChain{
		links: []forwardLink{
			{prepare: fake.Signature{}, commit: fake.Signature{}},
			{to: hash, prepare: fake.Signature{}, commit: fake.Signature{}},
		},
	}

	require.Equal(t, hash, chain.GetTo())
}

func TestChain_VisitJSON(t *testing.T) {
	chain := forwardLinkChain{
		links: []forwardLink{
			{
				from:    []byte{0xa},
				to:      []byte{0xb},
				prepare: fake.Signature{},
				commit:  fake.Signature{},
			},
		},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(chain)
	require.NoError(t, err)
	expected := `[{"From":"Cg==","To":"Cw==","Prepare":{},"Commit":{},"ChangeSet":null}]`
	require.Equal(t, expected, string(data))

	_, err = chain.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err,
		"couldn't serialize link: couldn't serialize prepare signature: fake error")
}

func TestChainFactory_VisitJSON(t *testing.T) {
	expected := forwardLinkChain{
		links: []forwardLink{
			{
				hash:      []byte{},
				from:      []byte{0xa},
				to:        []byte{0xb},
				prepare:   fake.Signature{},
				commit:    fake.Signature{},
				changeset: fake.Message{},
			},
			{
				hash:      []byte{},
				from:      []byte{0xb},
				to:        []byte{0xc},
				prepare:   fake.Signature{},
				commit:    fake.Signature{},
				changeset: fake.Message{},
			},
		},
	}

	factory := chainFactory{
		sigFactory:      fake.SignatureFactory{},
		csFactory:       fake.MessageFactory{},
		hashFactory:     fake.NewHashFactory(&fake.Hash{}),
		verifierFactory: fake.VerifierFactory{},
		viewchange:      fakeViewChange{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(expected)
	require.NoError(t, err)

	var chain consensus.Chain
	err = ser.Deserialize(data, factory, &chain)
	require.NoError(t, err)
	require.Equal(t, expected, chain)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize chain: fake error")

	input := fake.FactoryInput{
		Serde:   fake.NewBadSerializer(),
		Message: types.Chain{{}},
	}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize prepare: fake error")

	input.Serde = fake.NewBadSerializerWithDelay(1)
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize commit: fake error")

	input.Serde = fake.NewBadSerializerWithDelay(2)
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize change set: fake error")

	input.Serde = fake.Serializer{}
	factory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err,
		"couldn't compute hash: couldn't write 'from': fake error")

	factory.hashFactory = fake.NewHashFactory(&fake.Hash{})
	factory.verifierFactory = fake.NewBadVerifierFactory()
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err,
		"couldn't verify the chain: couldn't create the verifier: fake error")
}

func TestChainFactory_Verify(t *testing.T) {
	factory := chainFactory{
		verifierFactory: fake.VerifierFactory{},
		viewchange:      fakeViewChange{},
	}

	chain := forwardLinkChain{}

	// Empty chain is verified all the time.
	err := factory.verify(chain)
	require.NoError(t, err)

	// If the genesis authority cannot be read, it accepts the chain.
	// TODO: this should not be the case!
	chain.links = []forwardLink{{}}
	factory.viewchange = fakeViewChange{err: xerrors.New("oops")}
	err = factory.verify(chain)
	require.NoError(t, err)

	factory.viewchange = fakeViewChange{}
	factory.verifierFactory = fake.NewVerifierFactory(fake.NewBadVerifier())
	err = factory.verify(chain)
	require.EqualError(t, err,
		"couldn't verify link 0: couldn't verify prepare signature: fake error")

	chain.links = []forwardLink{
		{to: []byte{0xc}, prepare: fake.Signature{}},
		{from: []byte{0xb}, prepare: fake.Signature{}},
	}
	factory.verifierFactory = fake.VerifierFactory{}
	err = factory.verify(chain)
	require.EqualError(t, err, "mismatch forward link '0c' != '0b'")
}
