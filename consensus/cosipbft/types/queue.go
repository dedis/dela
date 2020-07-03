package types

import (
	"bytes"
	"sync"

	"go.dedis.ch/dela/crypto"
	"golang.org/x/xerrors"
)

// Queue is an interface specific to cosipbft that defines the primitives to
// prepare and commit to proposals.
type Queue interface {
	New(fl ForwardLink, authority crypto.CollectiveAuthority) error
	LockProposal(to Digest, sig crypto.Signature) error
	Finalize(to Digest, sig crypto.Signature) (*ForwardLink, error)
	Clear()
}

type item struct {
	ForwardLink
	verifier crypto.Verifier
}

type queue struct {
	sync.Mutex
	locked      bool
	hashFactory crypto.HashFactory
	verifierFac crypto.VerifierFactory
	items       []item
}

// NewQueue creates a new empty queue.
func NewQueue(fac crypto.VerifierFactory) Queue {
	return &queue{
		locked:      false,
		hashFactory: crypto.NewSha256Factory(),
		verifierFac: fac,
	}
}

func (q *queue) getItem(id Digest) (item, int, bool) {
	for i, item := range q.items {
		if bytes.Equal(id, item.to) {
			return item, i, true
		}
	}

	return item{}, -1, false
}

func (q *queue) New(fl ForwardLink, authority crypto.CollectiveAuthority) error {
	q.Lock()
	defer q.Unlock()

	if q.locked {
		return xerrors.New("queue is locked")
	}

	_, _, ok := q.getItem(fl.to)
	if ok {
		// Duplicate are ignored without triggering an error as the exact same
		// digest means they are exactly the same.
		return nil
	}

	verifier, err := q.verifierFac.FromAuthority(authority)
	if err != nil {
		return xerrors.Errorf("couldn't make verifier: %v", err)
	}

	q.items = append(q.items, item{
		ForwardLink: fl,
		verifier:    verifier,
	})
	return nil
}

// LockProposal verifies the prepare signature and stores it. It also locks
// the queue to prevent further committing.
func (q *queue) LockProposal(to Digest, sig crypto.Signature) error {
	q.Lock()
	defer q.Unlock()

	item, index, ok := q.getItem(to)
	if !ok {
		return xerrors.Errorf("couldn't find proposal '%x'", to)
	}

	if item.prepare != nil {
		// Signature already populated so a commit has already been received.
		return nil
	}

	if q.locked {
		return xerrors.New("queue is locked")
	}

	forwardLink := item
	forwardLink.prepare = sig

	h := q.hashFactory.New()
	err := forwardLink.Fingerprint(h)
	if err != nil {
		return xerrors.Errorf("couldn't hash proposal: %v", err)
	}

	err = item.verifier.Verify(h.Sum(nil), sig)
	if err != nil {
		return xerrors.Errorf("couldn't verify signature: %v", err)
	}

	q.items[index] = forwardLink
	q.locked = true

	return nil
}

// Finalize verifies the commit signature and clear the queue.
func (q *queue) Finalize(to Digest, sig crypto.Signature) (*ForwardLink, error) {
	q.Lock()
	defer q.Unlock()

	item, _, ok := q.getItem(to)
	if !ok {
		return nil, xerrors.Errorf("couldn't find proposal '%x'", to)
	}

	if item.prepare == nil {
		return nil, xerrors.Errorf("no signature for proposal '%x'", to)
	}

	forwardLink := item.ForwardLink
	forwardLink.commit = sig

	// Make sure the commit signature is a valid one before committing.
	buffer, err := forwardLink.prepare.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal the signature: %v", err)
	}

	err = item.verifier.Verify(buffer, sig)
	if err != nil {
		return nil, xerrors.Errorf("couldn't verify signature: %v", err)
	}

	q.locked = false
	q.items = nil

	return &forwardLink, nil
}

func (q *queue) Clear() {
	q.Lock()
	q.locked = false
	q.items = nil
	q.Unlock()
}
