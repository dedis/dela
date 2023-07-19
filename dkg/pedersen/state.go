package pedersen

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3"
	"golang.org/x/xerrors"
	"sync"
)

// dkgState represents the states of a DKG node. States change as follow:
//
//	┌───────┐          ┌───────┐
//	│Initial├─────────►│Sharing│
//	└───┬───┘          └───┬───┘
//	    │                  │
//	┌───▼─────┬─────►┌─────▼───┐
//	│Resharing│      │Certified│
//	└─────────┘◄─────┴─────────┘
type dkgState byte

func (s dkgState) String() string {
	switch s {
	case initial:
		return "Initial"
	case sharing:
		return "Sharing"
	case certified:
		return "Certified"
	case resharing:
		return "Resharing"
	default:
		return "UNKNOWN"
	}
}

const (
	initial dkgState = iota
	sharing
	certified
	resharing
)

// state is a struct contained in a handler that allows an actor to read the
// state of that handler. The actor should only use the getter functions to read
// the attributes.
type state struct {
	sync.Mutex
	// distrKey is only set once the node is certified
	distrKey kyber.Point
	// participants is set once a sharing or resharing starts
	participants []mino.Address
	pubkeys      []kyber.Point
	//TODO add verifiability (poly         *share.PubPoly)
	threshold int
	dkgState  dkgState
}

func (s *state) switchState(new dkgState) error {
	s.Lock()
	defer s.Unlock()

	current := s.dkgState

	switch new {
	case initial:
		return xerrors.Errorf("initial state cannot be set manually")
	case sharing:
		if current != initial {
			return xerrors.Errorf("sharing state must switch from initial: %s", current)
		}
	case certified:
		if current != sharing && current != resharing {
			return xerrors.Errorf("certified state must switch from sharing or resharing: %s", current)
		}
	case resharing:
		if current != initial && current != certified {
			return xerrors.Errorf("resharing state must switch from initial or certified: %s", current)
		}
	}

	s.dkgState = new

	return nil
}

func (s *state) checkState(states ...dkgState) error {
	s.Lock()
	defer s.Unlock()

	current := s.dkgState

	for _, state := range states {
		if state == current {
			return nil
		}
	}

	return xerrors.Errorf("unexpected state: %s != one of %v", current, states)
}

func (s *state) Done() bool {
	s.Lock()
	defer s.Unlock()

	current := s.dkgState
	return current == certified
}

func (s *state) init(participants []mino.Address, pubkeys []kyber.Point, t int) {
	s.Lock()
	defer s.Unlock()

	s.participants = participants
	s.pubkeys = pubkeys
	s.threshold = t
}

func (s *state) getDistKey() kyber.Point {
	s.Lock()
	defer s.Unlock()
	return s.distrKey
}

func (s *state) setDistKey(key kyber.Point) {
	s.Lock()
	s.distrKey = key
	s.Unlock()
}

func (s *state) getParticipants() []mino.Address {
	s.Lock()
	defer s.Unlock()
	return s.participants
}

func (s *state) getPublicKeys() []kyber.Point {
	s.Lock()
	defer s.Unlock()
	return s.pubkeys
}

func (s *state) getThreshold() int {
	s.Lock()
	defer s.Unlock()
	return s.threshold
}
