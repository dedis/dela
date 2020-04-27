package cosipbft

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/consensus/viewchange"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestQueue_New(t *testing.T) {
	prop := forwardLink{from: []byte{0xaa}, to: []byte{0xbb}}

	authority := fake.NewAuthority(3, fake.NewSigner)

	queue := &queue{cosi: &fakeCosi{}}
	err := queue.New(prop, authority)
	require.NoError(t, err)
	require.Len(t, queue.items, 1)
	require.Equal(t, prop.from, queue.items[0].from)
	require.Equal(t, prop.to, queue.items[0].to)
	require.NotNil(t, queue.items[0].verifier)

	err = queue.New(forwardLink{from: []byte{0xbb}}, authority)
	require.NoError(t, err)
	require.Len(t, queue.items, 2)
	require.Equal(t, prop.to, queue.items[1].from)

	err = queue.New(prop, authority)
	require.EqualError(t, err, "proposal 'bb' already exists")

	queue.locked = true
	err = queue.New(prop, authority)
	require.EqualError(t, err, "queue is locked")

	queue.locked = false
	queue.cosi = &fakeCosi{verifierFactory: fake.NewBadVerifierFactory()}
	prop.to = []byte{0xcc}
	err = queue.New(prop, authority)
	require.EqualError(t, err, "couldn't make verifier: fake error")
}

func TestQueue_LockProposal(t *testing.T) {
	verifier := &fakeVerifier{}
	queue := &queue{
		encoder:     encoding.NewProtoEncoder(),
		hashFactory: crypto.NewSha256Factory(),
		items: []item{
			{
				forwardLink: forwardLink{
					from: []byte{0xaa},
					to:   []byte{0xbb},
				},
				verifier: verifier,
			},
		},
	}

	err := queue.LockProposal([]byte{0xbb}, fake.Signature{})
	require.NoError(t, err)
	require.NotNil(t, queue.items[0].prepare)
	require.True(t, queue.locked)
	require.Len(t, verifier.calls, 1)

	forwardLink := forwardLink{from: []byte{0xaa}, to: []byte{0xbb}}
	hash, err := forwardLink.computeHash(sha256Factory{}.New(), encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.Equal(t, hash, verifier.calls[0]["message"])

	queue.locked = false
	err = queue.LockProposal([]byte{0xaa}, nil)
	require.EqualError(t, err, "couldn't find proposal 'aa'")

	queue.locked = false
	queue.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	err = queue.LockProposal([]byte{0xbb}, fake.Signature{})
	require.EqualError(t, err,
		"couldn't hash proposal: couldn't write 'from': fake error")

	queue.hashFactory = crypto.NewSha256Factory()
	verifier.err = xerrors.New("oops")
	err = queue.LockProposal([]byte{0xbb}, fake.Signature{})
	require.EqualError(t, err, "couldn't verify signature: oops")

	queue.locked = true
	err = queue.LockProposal([]byte{0xbb}, nil)
	require.EqualError(t, err, "queue is locked")
}

func TestQueue_Finalize(t *testing.T) {
	verifier := &fakeVerifier{}
	queue := &queue{
		encoder: encoding.NewProtoEncoder(),
		items: []item{
			{
				forwardLink: forwardLink{
					from:      []byte{0xaa},
					to:        []byte{0xbb},
					prepare:   fake.Signature{},
					changeset: viewchange.ChangeSet{},
				},
				verifier: verifier,
			},
		},
	}

	pb, err := queue.Finalize([]byte{0xbb}, fake.Signature{})
	require.NoError(t, err)
	require.NotNil(t, pb)
	require.False(t, queue.locked)
	require.Nil(t, queue.items)
	require.Len(t, verifier.calls, 1)
	require.Equal(t, []byte{fake.SignatureByte}, verifier.calls[0]["message"])

	_, err = queue.Finalize([]byte{0xaa}, nil)
	require.EqualError(t, err, "couldn't find proposal 'aa'")

	queue.items = []item{{forwardLink: forwardLink{to: []byte{0xaa}}}}
	_, err = queue.Finalize([]byte{0xaa}, nil)
	require.EqualError(t, err, "no signature for proposal 'aa'")

	queue.items[0].forwardLink = forwardLink{to: []byte{0xaa}, prepare: fake.NewBadSignature()}
	_, err = queue.Finalize([]byte{0xaa}, fake.Signature{})
	require.EqualError(t, err, "couldn't marshal the signature: fake error")

	queue.items = []item{
		{
			forwardLink: forwardLink{
				to:      []byte{0xaa},
				prepare: fake.Signature{},
			},
			verifier: &fakeVerifier{err: xerrors.New("oops")},
		},
	}
	_, err = queue.Finalize([]byte{0xaa}, fake.Signature{})
	require.EqualError(t, err, "couldn't verify signature: oops")

	queue.items[0].verifier = &fakeVerifier{}
	queue.encoder = fake.BadPackEncoder{}
	_, err = queue.Finalize([]byte{0xaa}, fake.NewBadSignature())
	require.EqualError(t, err, "couldn't pack forward link: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeVerifier struct {
	crypto.Verifier

	calls []map[string]interface{}
	err   error
	delay int
}

func (v *fakeVerifier) Verify(msg []byte, sig crypto.Signature) error {
	v.calls = append(v.calls, map[string]interface{}{
		"message":   msg,
		"signature": sig,
	})

	if v.err != nil {
		if v.delay == 0 {
			return v.err
		}
		v.delay--
	}

	return nil
}
