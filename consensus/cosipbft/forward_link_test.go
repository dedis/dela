package cosipbft

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestChain_GetLast(t *testing.T) {
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
}
