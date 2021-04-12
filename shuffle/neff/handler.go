package neff

import (
	"context"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/shuffle/neff/types"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/proof"
	pedersen "go.dedis.ch/kyber/v3/share/dkg/pedersen"
	shuffleKyber "go.dedis.ch/kyber/v3/shuffle"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
	"sync"
	"time"
)

// Todo :  use a timeout, the maximum time a node will wait for a response
const recvResponseTimeout = time.Second * 10

// Todo : either add some sort of setup call to set participants or public key (and thus reduce message size ) or remove state
// state is a struct contained in a handler that allows an actor to read the
// state of that handler. The actor should only use the getter functions to read
// the attributes.
type state struct {
	sync.Mutex
	//distrKey     kyber.Point
	participants []mino.Address
}

// Handler represents the RPC executed on each node
//
// - implements mino.Handler
type Handler struct {
	mino.UnsupportedHandler
	sync.RWMutex
	dkg       *pedersen.DistKeyGenerator
	me        mino.Address
	startRes  *state
}

// NewHandler creates a new handler
func NewHandler(me mino.Address) *Handler {
	return &Handler{
		me:       me,
		startRes: &state{},
	}
}

// Stream implements mino.Handler. It allows one to stream messages to the
// players.
func (h *Handler) Stream(out mino.Sender, in mino.Receiver) error {

	from, msg, err := in.Recv(context.Background())
	if err != nil {
		return xerrors.Errorf("failed to receive: %v", err)
	}

	switch msg := msg.(type) {

	case types.ShuffleMessage:
		err := h.HandleShuffleMessage(msg, from, out, in)
		if err != nil {
			return xerrors.Errorf("failed to start: %v", err)
		}

	default:
		return xerrors.Errorf("expected Start message, decrypt request or "+
			"Deal as first message, got: %T", msg)
	}

	return nil
}

// Todo : handle edge cases
func (h *Handler) HandleShuffleMessage (shuffleMessage types.ShuffleMessage, from mino.Address, out mino.Sender,
	in mino.Receiver) error {

	dela.Logger.Info().Msg("SHUFFLE / RECEIVED FROM  : " + from.String())

	addrs := shuffleMessage.GetAddresses()
	suite := suites.MustFind(shuffleMessage.GetSuiteName())
	publicKey := shuffleMessage.GetPublicKey()
	kBar := shuffleMessage.GetkBar()
	cBar := shuffleMessage.GetcBar()
	kBarPrevious := shuffleMessage.GetkBarPrevious()
	cBarPrevious := shuffleMessage.GetcBarPrevious()
	prf := shuffleMessage.GetProof()

	// leader node
	if addrs[0].Equal(h.me) {
		dela.Logger.Info().Msg("SHUFFLE / SENDING TO : " + addrs[1].String())
		errs := out.Send(shuffleMessage, addrs[1])
		err := <-errs
		if err != nil {
			return xerrors.Errorf("failed to send Shuffle Message: %v", err)
		}

		lastNodeAddress, msg, err := in.Recv(context.Background())
		dela.Logger.Info().Msg("RECEIVED FROM  : " + lastNodeAddress.String())
		errs = out.Send(msg, from)
		err = <-errs
		if err != nil {
			return xerrors.Errorf("failed to send Shuffle Message: %v", err)
		}
		return nil
	}

	//Todo : check you received from the correct node

	err := verify(suite, kBarPrevious, cBarPrevious, publicKey, kBar, cBar, prf)
	if err != nil {
		return xerrors.Errorf("Shuffle verify failed: %v", err)
	}

	rand := suite.RandomStream()
	KbarNext, CbarNext, prover := shuffleKyber.Shuffle(suite, nil, publicKey, kBar, cBar, rand)
	prfNext, err := proof.HashProve(suite, protocolName, prover)
	if err != nil {
		return xerrors.Errorf("Shuffle proof failed: ", err.Error())
	}

	message := types.NewShuffleMessage(addrs, shuffleMessage.GetSuiteName(), publicKey, KbarNext,
		CbarNext, kBar, cBar, prfNext)

	index := 0
	for i, addr := range addrs {
		if addr.Equal(from) {
			index = i
			break
		}
	}
	//todo : use modulo
	index += 2

	if index >= len(addrs) {
		index = 0
	}

	dela.Logger.Info().Msg("SHUFFLE / SENDING TO : " + addrs[index].String())

	errs := out.Send(message, addrs[index])
	err = <-errs
	if err != nil {
		return xerrors.Errorf("failed to send Shuffle Message: %v", err)
	}

	return nil
}

func verify (suite suites.Suite, Ks []kyber.Point, Cs []kyber.Point,
	pubKey kyber.Point, KsShuffled []kyber.Point, CsShuffled []kyber.Point, prf []byte) (err error) {

	verifier := shuffleKyber.Verifier(suite, nil, pubKey, Ks, Cs, KsShuffled, CsShuffled)
	return proof.HashVerify(suite, protocolName, verifier, prf)

}

