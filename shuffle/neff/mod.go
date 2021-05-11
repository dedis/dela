package neff

import (
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/txn/pool"
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
	"strconv"
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
	service ordering.Service
	p       pool.Pool
	blocks  *blockstore.InDisk
}

// NewNeffShuffle returns a new NeffShuffle factory.
func NewNeffShuffle(m mino.Mino, s ordering.Service, p pool.Pool, blocks *blockstore.InDisk) *NeffShuffle {
	factory := types.NewMessageFactory(m.GetAddressFactory())

	return &NeffShuffle{
		mino:    m,
		factory: factory,
		service: s,
		p :      p,
		blocks:  blocks,
	}
}

// Listen implements shuffle.SHUFFLE. It must be called on each node that participates
// in the SHUFFLE. Creates the RPC.
func (n NeffShuffle) Listen() (shuffle.Actor, error) {
	h := NewHandler(n.mino.GetAddress(), n.service, n.p, n.blocks)

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
	mino     mino.Mino
	factory  serde.Factory
	startRes *state
}

// Shuffle must be called by ONE of the actor to shuffle the list of ElGamal pairs.
// Each node represented by a player must first execute Listen().
func (a Actor) Shuffle(co crypto.CollectiveAuthority, electionId string) (err error) {

	ctx, cancel := context.WithTimeout(context.Background(), shuffleTimeout)
	defer cancel()

	sender, _, err := a.rpc.Stream(ctx, co)
	if err != nil {
		return xerrors.Errorf("failed to stream: %v", err)
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

	threshold := len(addrs)
	message := types.NewStartShuffle(threshold, electionId, addrs)
	dela.Logger.Info().Msg( "threshold: "  + strconv.Itoa(threshold))

	errs := sender.Send(message, addrs...)
	err = <-errs
	if err != nil {
		return xerrors.Errorf("failed to send first message: %v", err)
	}

	return nil
}

// Verify allows to verify a Shuffle
func (a Actor) Verify(suiteName string, Ks []kyber.Point, Cs []kyber.Point,
	pubKey kyber.Point, KsShuffled []kyber.Point, CsShuffled []kyber.Point, prf []byte) (err error) {

	suite := suites.MustFind(suiteName)

	verifier := shuffleKyber.Verifier(suite, nil, pubKey, Ks, Cs, KsShuffled, CsShuffled)
	return proof.HashVerify(suite, protocolName, verifier, prf)

}


