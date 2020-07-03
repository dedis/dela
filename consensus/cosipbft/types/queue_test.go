package types

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestQueue_New(t *testing.T) {
	prop := ForwardLink{from: []byte{0xaa}, to: []byte{0xbb}}

	authority := fake.NewAuthority(3, fake.NewSigner)

	queue := NewQueue(fake.VerifierFactory{}).(*queue)
	err := queue.New(prop, authority)
	require.NoError(t, err)
	err = queue.New(prop, authority)
	require.NoError(t, err)

	require.Len(t, queue.items, 1)
	require.Equal(t, prop.from, queue.items[0].from)
	require.Equal(t, prop.to, queue.items[0].to)
	require.NotNil(t, queue.items[0].verifier)

	err = queue.New(ForwardLink{from: []byte{0xbb}}, authority)
	require.NoError(t, err)
	require.Len(t, queue.items, 2)
	require.Equal(t, prop.to, queue.items[1].from)

	queue.locked = true
	err = queue.New(prop, authority)
	require.EqualError(t, err, "queue is locked")

	queue.locked = false
	queue.verifierFac = fake.NewBadVerifierFactory()
	prop.to = []byte{0xcc}
	err = queue.New(prop, authority)
	require.EqualError(t, err, "couldn't make verifier: fake error")
}

func TestQueue_LockProposal(t *testing.T) {
	verifier := &fakeVerifier{}
	queue := &queue{
		hashFactory: crypto.NewSha256Factory(),
		items: []item{
			{
				ForwardLink: ForwardLink{
					from: []byte{0xaa},
					to:   []byte{0xbb},
				},
				verifier: verifier,
			},
			{
				ForwardLink: ForwardLink{to: []byte{0xcc}},
				verifier:    verifier,
			},
		},
	}

	err := queue.LockProposal([]byte{0xbb}, fake.Signature{})
	require.NoError(t, err)
	err = queue.LockProposal([]byte{0xbb}, fake.Signature{})
	require.NoError(t, err)

	require.NotNil(t, queue.items[0].prepare)
	require.True(t, queue.locked)
	require.Len(t, verifier.calls, 1)

	forwardLink := ForwardLink{from: []byte{0xaa}, to: []byte{0xbb}}
	h := crypto.NewSha256Factory().New()
	err = forwardLink.Fingerprint(h)
	require.NoError(t, err)
	require.Equal(t, h.Sum(nil), verifier.calls[0]["message"])

	queue.locked = false
	err = queue.LockProposal([]byte{0xaa}, nil)
	require.EqualError(t, err, "couldn't find proposal 'aa'")

	queue.locked = false
	queue.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	err = queue.LockProposal([]byte{0xcc}, fake.Signature{})
	require.EqualError(t, err,
		"couldn't hash proposal: couldn't write 'from': fake error")

	queue.hashFactory = crypto.NewSha256Factory()
	verifier.err = xerrors.New("oops")
	err = queue.LockProposal([]byte{0xcc}, fake.Signature{})
	require.EqualError(t, err, "couldn't verify signature: oops")

	queue.locked = true
	err = queue.LockProposal([]byte{0xcc}, nil)
	require.EqualError(t, err, "queue is locked")
}

func TestQueue_Finalize(t *testing.T) {
	verifier := &fakeVerifier{}
	queue := &queue{
		items: []item{
			{
				ForwardLink: ForwardLink{
					from:      []byte{0xaa},
					to:        []byte{0xbb},
					prepare:   fake.Signature{},
					changeset: fake.Message{},
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

	queue.items = []item{{ForwardLink: ForwardLink{to: []byte{0xaa}}}}
	_, err = queue.Finalize([]byte{0xaa}, nil)
	require.EqualError(t, err, "no signature for proposal 'aa'")

	queue.items[0].ForwardLink = ForwardLink{to: []byte{0xaa}, prepare: fake.NewBadSignature()}
	_, err = queue.Finalize([]byte{0xaa}, fake.Signature{})
	require.EqualError(t, err, "couldn't marshal the signature: fake error")

	queue.items = []item{
		{
			ForwardLink: ForwardLink{
				to:      []byte{0xaa},
				prepare: fake.Signature{},
			},
			verifier: &fakeVerifier{err: xerrors.New("oops")},
		},
	}
	_, err = queue.Finalize([]byte{0xaa}, fake.Signature{})
	require.EqualError(t, err, "couldn't verify signature: oops")
}

func TestQueue_Clear(t *testing.T) {
	queue := &queue{
		locked: true,
		items:  []item{{}, {}},
	}

	queue.Clear()
	require.False(t, queue.locked)
	require.Empty(t, queue.items)
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
