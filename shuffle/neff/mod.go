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
	"sync"
	"time"
)

const (
	shuffleTimeout = time.Second * 300
	protocolName   = "PairShuffle"
)

// NeffShuffle allows one to initialize a new SHUFFLE protocol.
//
// - implements shuffle.SHUFFLE
type NeffShuffle struct {
	mino    mino.Mino
	factory serde.Factory
	service ordering.Service
	p       pool.Pool
	blocks  blockstore.BlockStore
	signer  crypto.Signer
}

// NewNeffShuffle returns a new NeffShuffle factory.
func NewNeffShuffle(m mino.Mino, s ordering.Service, p pool.Pool, blocks blockstore.BlockStore, signer crypto.Signer) *NeffShuffle {
	factory := types.NewMessageFactory(m.GetAddressFactory())

	return &NeffShuffle{
		mino:    m,
		factory: factory,
		service: s,
		p:       p,
		blocks:  blocks,
		signer:  signer,
	}
}

// Listen implements shuffle.SHUFFLE. It must be called on each node that
// participates in the SHUFFLE. Creates the RPC.
func (n NeffShuffle) Listen() (shuffle.Actor, error) {
	h := NewHandler(n.mino.GetAddress(), n.service, n.p, n.blocks, n.signer)

	a := &Actor{
		rpc:     mino.MustCreateRPC(n.mino, "shuffle", h, n.factory),
		factory: n.factory,
		mino:    n.mino,
	}

	return a, nil
}

// Actor allows one to perform SHUFFLE operations like shuffling a list of
// ElGamal pairs and verify a shuffle
//
// - implements shuffle.Actor
type Actor struct {
	sync.Mutex
	rpc     mino.RPC
	mino    mino.Mino
	factory serde.Factory
	// startRes *state
}

// Shuffle must be called by ONE of the actor to shuffle the list of ElGamal
// pairs.
// Each node represented by a player must first execute Listen().
func (a *Actor) Shuffle(co crypto.CollectiveAuthority, electionId string) (err error) {

	a.Lock()
	defer a.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), shuffleTimeout)
	defer cancel()

	sender, receiver, err := a.rpc.Stream(ctx, co)
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
	dela.Logger.Info().Msgf("threshold: %d", threshold)

	errs := sender.Send(message, addrs...)
	err = <-errs
	if err != nil {
		return xerrors.Errorf("failed to send first message: %v", err)
	}

	// todo add timeout, ask noémien and gaurav about every timeout
	addr, msg, err := receiver.Recv(context.Background())

	if err != nil {
		return xerrors.Errorf("got an error from '%s' while "+
			"receiving: %v", addr, err)
	}
	_, ok := msg.(types.EndShuffle)
	if !ok {
		cancel()
		return xerrors.Errorf("expected to receive an EndShuffle message, but "+
			"go the following: %T", msg)
	}

	return nil
}

// Todo : this is useless in the new implementation, maybe remove ?
// Verify allows to verify a Shuffle
func (a *Actor) Verify(suiteName string, Ks []kyber.Point, Cs []kyber.Point,
	pubKey kyber.Point, KsShuffled []kyber.Point, CsShuffled []kyber.Point, prf []byte) (err error) {

	suite := suites.MustFind(suiteName)

	verifier := shuffleKyber.Verifier(suite, nil, pubKey, Ks, Cs, KsShuffled, CsShuffled)
	return proof.HashVerify(suite, protocolName, verifier, prf)

}
