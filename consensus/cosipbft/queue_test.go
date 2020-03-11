package cosipbft

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/crypto"
	"golang.org/x/xerrors"
)

func TestQueue_New(t *testing.T) {
	prev := fakeItem{hash: []byte{0xaa}}
	curr := fakeItem{hash: []byte{0xbb}}

	queue := &queue{}
	err := queue.New(curr, prev)
	require.NoError(t, err)
	require.Len(t, queue.items, 1)
	require.Equal(t, prev.hash, queue.items[0].from)
	require.Equal(t, curr.hash, queue.items[0].to)

	err = queue.New(prev, curr)
	require.NoError(t, err)
	require.Len(t, queue.items, 2)
	require.Equal(t, curr.hash, queue.items[1].from)
	require.Equal(t, prev.hash, queue.items[1].to)

	err = queue.New(curr, prev)
	require.EqualError(t, err, "proposal 'bb' already exists")

	queue.locked = true
	err = queue.New(curr, prev)
	require.EqualError(t, err, "queue is locked")
}

// fakeQueueFactory is only used to return a specific hash factory.
type fakeQueueFactory struct {
	ChainFactory
	bad bool
}

func (f fakeQueueFactory) GetHashFactory() crypto.HashFactory {
	if f.bad {
		return badHashFactory{}
	}
	return sha256Factory{}
}

func TestQueue_LockProposal(t *testing.T) {
	verifier := &fakeVerifier{}
	queue := &queue{
		chainFactory: fakeQueueFactory{},
		verifier:     verifier,
		items: []item{
			{
				from:       []byte{0xaa},
				to:         []byte{0xbb},
				publicKeys: []crypto.PublicKey{},
			},
		},
	}

	err := queue.LockProposal([]byte{0xbb}, fakeSignature{})
	require.NoError(t, err)
	require.NotNil(t, queue.items[0].prepare)
	require.True(t, queue.locked)
	require.Len(t, verifier.calls, 1)

	forwardLink := forwardLink{from: []byte{0xaa}, to: []byte{0xbb}}
	hash, err := forwardLink.computeHash(sha256Factory{}.New())
	require.NoError(t, err)
	require.Equal(t, hash, verifier.calls[0]["message"])

	queue.locked = false
	err = queue.LockProposal([]byte{0xaa}, nil)
	require.EqualError(t, err, "couldn't find proposal 'aa'")

	queue.locked = false
	queue.chainFactory = fakeQueueFactory{bad: true}
	err = queue.LockProposal([]byte{0xbb}, fakeSignature{})
	require.EqualError(t, err, "couldn't hash proposal: couldn't write from: oops")

	queue.chainFactory = fakeQueueFactory{}
	queue.verifier = &fakeVerifier{err: xerrors.New("oops")}
	err = queue.LockProposal([]byte{0xbb}, fakeSignature{})
	require.EqualError(t, err, "couldn't verify signature: oops")

	queue.locked = true
	err = queue.LockProposal([]byte{0xbb}, nil)
	require.EqualError(t, err, "queue is locked")
}

func TestQueue_Finalize(t *testing.T) {
	verifier := &fakeVerifier{}
	queue := &queue{
		verifier: verifier,
		items: []item{
			{
				from:    []byte{0xaa},
				to:      []byte{0xbb},
				prepare: fakeSignature{},
			},
		},
	}

	pb, err := queue.Finalize([]byte{0xbb}, fakeSignature{})
	require.NoError(t, err)
	require.NotNil(t, pb)
	require.False(t, queue.locked)
	require.Nil(t, queue.items)
	require.Len(t, verifier.calls, 1)
	require.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, verifier.calls[0]["message"])

	_, err = queue.Finalize([]byte{0xaa}, nil)
	require.EqualError(t, err, "couldn't find proposal 'aa'")

	queue.items = []item{{to: []byte{0xaa}}}
	_, err = queue.Finalize([]byte{0xaa}, nil)
	require.EqualError(t, err, "no signature for proposal 'aa'")

	queue.items = []item{{to: []byte{0xaa}, prepare: fakeSignature{err: xerrors.New("oops")}}}
	_, err = queue.Finalize([]byte{0xaa}, fakeSignature{})
	require.EqualError(t, err, "couldn't marshal the signature: oops")

	queue.items = []item{{to: []byte{0xaa}, prepare: fakeSignature{}}}
	queue.verifier = &fakeVerifier{err: xerrors.New("oops")}
	_, err = queue.Finalize([]byte{0xaa}, fakeSignature{})
	require.EqualError(t, err, "couldn't verify signature: oops")

	queue.verifier = &fakeVerifier{}
	_, err = queue.Finalize([]byte{0xaa}, fakeSignature{err: xerrors.New("oops")})
	require.EqualError(t, xerrors.Unwrap(xerrors.Unwrap(err)), "couldn't pack: oops")
}

type fakeItem struct {
	consensus.Proposal
	hash []byte
}

func (i fakeItem) GetHash() []byte {
	return i.hash
}

func (i fakeItem) GetPublicKeys() []crypto.PublicKey {
	return nil
}

type fakeVerifier struct {
	crypto.Verifier

	calls []map[string]interface{}
	err   error
	delay int
}

func (v *fakeVerifier) Verify(pubkeys []crypto.PublicKey, msg []byte, sig crypto.Signature) error {
	v.calls = append(v.calls, map[string]interface{}{
		"pubkeys":   pubkeys,
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
