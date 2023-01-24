package pedersen

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"go.dedis.ch/kyber/v3"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

// the receiving time out, after which we check if the DKG setup is done or not.
// Allows to exit the loop.
var recvTimeout = time.Second * 30

// constant used in the logs
const newState = "new state"
const badState = "bad state: %v"
const failedState = "failed to switch state: %v"

type nodeType byte

// enumeration of the node type
const (
	unknownNode nodeType = iota
	oldNode
	commonNode
	newNode
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
	threshold    int
	dkgState     dkgState
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

// Handler represents the RPC executed on each node
//
// - implements mino.Handler
type Handler struct {
	mino.UnsupportedHandler
	sync.RWMutex
	log zerolog.Logger

	dkgHandler dkgHandler
}

// NewHandler creates a new handler
func NewHandler(privKey kyber.Scalar, me mino.Address) *Handler {
	log := dela.Logger.With().Str("role", "DKG handler").Str("addr", me.String()).Logger()

	return &Handler{
		log: log,

		dkgHandler: newSimpleHandler(log, me, privKey),
	}
}

// Stream implements mino.Handler. It allows one to stream messages to the
// players.
func (h *Handler) Stream(out mino.Sender, in mino.Receiver) error {
	// Note: one should never assume any synchronous properties on the messages.
	// For example we can not expect to receive the start message from the
	// initiator of the DKG protocol first because some node could have received
	// this start message earlier than us, start their DKG work by sending
	// messages to the other nodes, and then we might get their messages before
	// the start message.

	h.log.Info().Msg("stream start")

	globalCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		ctx, cancel := context.WithTimeout(context.Background(), recvTimeout)
		from, msg, err := in.Recv(ctx)
		cancel()

		if errors.Is(err, context.DeadlineExceeded) {
			if !h.dkgHandler.isRunning() {
				h.log.Info().Msg("stream done, deadline exceeded")
				return nil
			}
			continue
		}

		if errors.Is(err, io.EOF) {
			h.log.Info().Msg("stream done, EOF")
			return nil
		}

		if err != nil {
			return xerrors.Errorf("failed to receive: %v", err)
		}

		h.log.Info().Str("from", from.String()).Str("type", fmt.Sprintf("%T", msg)).
			Msg("message received")

		err = h.dkgHandler.handleMessage(globalCtx, msg, from, out)
		if err != nil {
			return xerrors.Errorf("failed to handle message: %v", err)
		}
	}
}

// checkEncryptionProof verifies the encryption proofs.
//
// See https://arxiv.org/pdf/2205.08529.pdf / section 5.4 Protocol / step 3
func checkEncryptionProof(cp types.Ciphertext) error {

	tmp1 := suite.Point().Mul(cp.F, nil)
	tmp2 := suite.Point().Mul(cp.E, cp.K)
	w := suite.Point().Sub(tmp1, tmp2)

	tmp1 = suite.Point().Mul(cp.F, cp.GBar)
	tmp2 = suite.Point().Mul(cp.E, cp.UBar)
	wBar := suite.Point().Sub(tmp1, tmp2)

	hash := sha256.New()
	cp.C.MarshalTo(hash)
	cp.K.MarshalTo(hash)
	cp.UBar.MarshalTo(hash)
	w.MarshalTo(hash)
	wBar.MarshalTo(hash)

	tmp := suite.Scalar().SetBytes(hash.Sum(nil))
	if !tmp.Equal(cp.E) {
		return xerrors.Errorf("hash not valid: %x != %x", cp.E, tmp)
	}

	return nil
}

// verifiableDecryption generates the decryption shares as well as the
// decryption proof.
//
// See https://arxiv.org/pdf/2205.08529.pdf / section 5.4 Protocol / step 3
func verifiableDecryption(ct types.Ciphertext, V kyber.Scalar, I int) (*types.ShareAndProof, error) {
	err := checkEncryptionProof(ct)
	if err != nil {
		return nil, xerrors.Errorf("failed to check proof: %v", err)
	}

	ui := suite.Point().Mul(V, ct.K)

	// share of this party, needed for decrypting
	partial := suite.Point().Sub(ct.C, ui)

	si := suite.Scalar().Pick(suite.RandomStream())
	UHat := suite.Point().Mul(si, ct.K)
	HHat := suite.Point().Mul(si, nil)

	hash := sha256.New()
	ui.MarshalTo(hash)
	UHat.MarshalTo(hash)
	HHat.MarshalTo(hash)
	Ei := suite.Scalar().SetBytes(hash.Sum(nil))

	Fi := suite.Scalar().Add(si, suite.Scalar().Mul(Ei, V))
	Hi := suite.Point().Mul(V, nil)

	sp := types.ShareAndProof{
		V:  partial,
		I:  int64(I),
		Ui: ui,
		Ei: Ei,
		Fi: Fi,
		Hi: Hi,
	}

	return &sp, nil
}

// isInSlice gets an address and a slice of addresses and returns true if that
// address is in the slice. This function is called for checking whether an old
// committee member is in the new committee as well or not
func isInSlice(addr mino.Address, addrs []mino.Address) bool {
	for _, other := range addrs {
		if addr.Equal(other) {
			return true
		}
	}

	return false
}

// union performs a union of el1 and el2.
func union(el1 []mino.Address, el2 []mino.Address) []mino.Address {
	addrsAll := el1

	for _, other := range el2 {
		exist := false
		for _, addr := range el1 {
			if addr.Equal(other) {
				exist = true
				break
			}
		}
		if !exist {
			addrsAll = append(addrsAll, other)
		}
	}

	return addrsAll
}
