package neff

import (
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/shuffle"
	"go.dedis.ch/dela/shuffle/neff/types"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/proof"
	shuffleKyber "go.dedis.ch/kyber/v3/shuffle"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/net/context"
	"golang.org/x/xerrors"
	"time"
)

const (
	shuffleTimeout   = time.Second * 300
	protocolName = "PairShuffle"
)

// NeffShuffle allows one to initialize a new SHUFFLE protocol.
//
// - implements shuffle.SHUFFLE
type NeffShuffle struct {
	mino    mino.Mino
	factory serde.Factory
}

// NewNeffShuffle returns a new NeffShuffle factory.
func NewNeffShuffle(m mino.Mino) *NeffShuffle {
	factory := types.NewMessageFactory(m.GetAddressFactory())

	return &NeffShuffle{
		mino:    m,
		factory: factory,
	}
}

// Listen implements shuffle.SHUFFLE. It must be called on each node that participates
// in the SHUFFLE. Creates the RPC.
func (n NeffShuffle) Listen() (shuffle.Actor, error) {
	h := NewHandler(n.mino.GetAddress())

	a := &Actor{
		rpc:      mino.MustCreateRPC(n.mino, "shuffle", h, n.factory),
		factory:  n.factory,
		startRes: h.startRes,
		mino:    n.mino,
	}

	return a, nil
}

// Actor allows one to perform SHUFFLE operations like shuffling a list of ElGamal pairs
// and verify a shuffle
//
// - implements shuffle.Actor
type Actor struct {
	rpc      mino.RPC
	mino    mino.Mino
	factory  serde.Factory
	startRes *state
}

// Shuffle must be called by ONE of the actor to shuffle the list of ElGamal pairs.
// Each node represented by a player must first execute Listen().
func (a Actor) Shuffle(co crypto.CollectiveAuthority, suiteName string, Ks []kyber.Point,
	Cs []kyber.Point, pubKey kyber.Point) (KsShuffled []kyber.Point,
	CsShuffled []kyber.Point, prf []byte, err error) {

	suite := suites.MustFind(suiteName)

	ctx, cancel := context.WithTimeout(context.Background(), shuffleTimeout)
	defer cancel()

	sender, receiver, err := a.rpc.Stream(ctx, co)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("failed to stream: %v", err)
	}

	addrs := make([]mino.Address, 0, co.Len())
	addrs = append(addrs, a.mino.GetAddress())

	addrIter := co.AddressIterator()

	for addrIter.HasNext() {
		addr := addrIter.GetNext()
		if !addr.Equal(a.mino.GetAddress()) {
			addrs = append(addrs, addr)
		}
	}

	rand := suite.RandomStream()
	Kbar, Cbar, prover := shuffleKyber.Shuffle(suite, nil, pubKey, Ks, Cs, rand)
	shuffleProof, err := proof.HashProve(suite, protocolName, prover)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("Shuffle proof failed: ", err.Error())
	}

	message := types.NewShuffleMessage(addrs, suiteName, pubKey, Kbar, Cbar, Ks, Cs, shuffleProof)

	errs := sender.Send(message, addrs[0])
	err = <-errs
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("failed to send first message: %v", err)
	}

	dela.Logger.Info().Msg("Starting the neff shuffle protocol ...")

	addr, msg, err := receiver.Recv(context.Background())

	dela.Logger.Info().Msg("End of the shuffle protocol.")

	if err != nil {
		return nil, nil, nil, xerrors.Errorf("got an error from '%s' while "+
			"receiving: %v", addr, err)
	}

	shuffleMsg, ok := msg.(types.ShuffleMessage)
	if !ok {
		return nil, nil, nil, xerrors.Errorf("expected to receive a Shuffle message, but "+
			"go the following: %T", msg)
	}

	kBar := shuffleMsg.GetkBar()
	cBar := shuffleMsg.GetcBar()
	finalProof := shuffleMsg.GetProof()

	kBarPrevious := shuffleMsg.GetkBarPrevious()
	cBarPrevious := shuffleMsg.GetcBarPrevious()

	err = a.Verify(suiteName, kBarPrevious, cBarPrevious, pubKey, kBar, cBar, finalProof)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("Shuffle verify failed: %v", err)
	}

	return kBar, cBar, finalProof, nil

}

// Verify allows to verify a Shuffle
func (a Actor) Verify(suiteName string, Ks []kyber.Point, Cs []kyber.Point,
	pubKey kyber.Point, KsShuffled []kyber.Point, CsShuffled []kyber.Point, prf []byte) (err error) {

	suite := suites.MustFind(suiteName)

	verifier := shuffleKyber.Verifier(suite, nil, pubKey, Ks, Cs, KsShuffled, CsShuffled)
	return proof.HashVerify(suite, protocolName, verifier, prf)

}


