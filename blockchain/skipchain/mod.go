// Package skipchain implements the blockchain interface by using the skipchain
// design, e.i. blocks are linked by one or several forward links collectively
// signed by the participants.
//
// TODO: think about versioning for upgradability.
package skipchain

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/rs/zerolog"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/blockchain/viewchange"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/consensus/cosipbft"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

const (
	// defaultPropagationTimeout is the default maximum amount of time given to
	// a propagation to reach every player.
	defaultPropogationTimeout = 30 * time.Second
)

// Skipchain is a blockchain that is using collective signatures to create links
// between the blocks.
//
// - implements blockchain.Blockchain
// - implements fmt.Stringer
type Skipchain struct {
	logger     zerolog.Logger
	mino       mino.Mino
	cosi       cosi.CollectiveSigning
	db         Database
	consensus  consensus.Consensus
	watcher    blockchain.Observable
	viewchange viewchange.ViewChange
}

// NewSkipchain returns a new instance of Skipchain.
func NewSkipchain(m mino.Mino, cosi cosi.CollectiveSigning) *Skipchain {
	consensus := cosipbft.NewCoSiPBFT(m, cosi)
	db := NewInMemoryDatabase()

	return &Skipchain{
		logger:    fabric.Logger,
		mino:      m,
		cosi:      cosi,
		db:        db,
		consensus: consensus,
		watcher:   blockchain.NewWatcher(),
	}
}

// Listen implements blockchain.Blockchain. It registers the RPC and starts the
// consensus module.
func (s *Skipchain) Listen(v blockchain.PayloadProcessor) (blockchain.Actor, error) {
	s.viewchange = viewchange.NewConstant(s.mino.GetAddress(), s)

	actor := skipchainActor{
		Skipchain:   s,
		hashFactory: sha256Factory{},
		rand:        crypto.CryptographicRandomGenerator{},
	}

	var err error
	actor.rpc, err = s.mino.MakeRPC("skipchain", newHandler(s))
	if err != nil {
		return nil, xerrors.Errorf("couldn't create the rpc: %v", err)
	}

	actor.consensus, err = s.consensus.Listen(newBlockValidator(s, v, s.watcher))
	if err != nil {
		return nil, xerrors.Errorf("couldn't start the consensus: %v", err)
	}

	return actor, nil
}

// GetBlockFactory implements blockchain.Blockchain. It returns the block
// factory for skipchains.
func (s *Skipchain) GetBlockFactory() blockchain.BlockFactory {
	return blockFactory{
		Skipchain:   s,
		hashFactory: sha256Factory{},
	}
}

// GetBlock implements blockchain.Blockchain. It returns the latest block.
func (s *Skipchain) GetBlock() (blockchain.Block, error) {
	block, err := s.db.ReadLast()
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the latest block: %v", err)
	}

	return block, nil
}

// GetVerifiableBlock implements blockchain.Blockchain. It reads the latest
// block of the chain and creates a verifiable proof of the shortest chain from
// the genesis to the block.
func (s *Skipchain) GetVerifiableBlock() (blockchain.VerifiableBlock, error) {
	block, err := s.db.ReadLast()
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the latest block: %v", err)
	}

	chain, err := s.consensus.GetChain(block.GetHash())
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the chain: %v", err)
	}

	vb := VerifiableBlock{
		SkipBlock: block,
		Chain:     chain,
	}

	return vb, nil
}

// Watch implements blockchain.Blockchain. It registers the observer so that it
// will be notified of new blocks. The caller is responsible for cancelling the
// context when the work is done.
func (s *Skipchain) Watch(ctx context.Context) <-chan blockchain.Block {
	ch := make(chan blockchain.Block, 10)
	obs := skipchainObserver{ch: ch}

	s.watcher.Add(obs)

	// Go routine to listen to the context cancel event. When it occurs, the
	// observer will be removed.
	go func() {
		<-ctx.Done()
		s.watcher.Remove(obs)
		close(ch)
	}()

	return ch
}

// String implements fmt.Stringer. It returns a simple representation of the
// skipchain instance to easily identify it.
func (s *Skipchain) String() string {
	return fmt.Sprintf("skipchain@%v", s.mino.GetAddress())
}

// skipchainActor provides the primitives of a blockchain actor.
//
// - implements blockchain.Actor
type skipchainActor struct {
	*Skipchain
	hashFactory crypto.HashFactory
	rand        crypto.RandGenerator
	consensus   consensus.Actor
	rpc         mino.RPC
}

// InitChain implements blockchain.Actor. It creates a genesis block if none
// exists and propagate it to the conodes.
func (a skipchainActor) InitChain(data proto.Message, players mino.Players) error {
	ca, ok := players.(cosi.CollectiveAuthority)
	if !ok {
		return xerrors.New("players must implement cosi.CollectiveAuthority")
	}

	_, err := a.db.Read(0)
	if err == nil {
		// Genesis block already exists.
		return nil
	}

	if !xerrors.Is(err, NewNoBlockError(0)) {
		return xerrors.Errorf("couldn't read the genesis block: %v", err)
	}

	conodes := newConodes(ca)
	iter := conodes.AddressIterator()

	if conodes.Len() > 0 && iter.GetNext().Equal(a.mino.GetAddress()) {
		// Only the first player tries to create the genesis block and then
		// propagates it to the other players.
		// This is done only once for a new chain thus we can assume that the
		// first one will be online at that moment.
		err := a.newChain(data, conodes)
		if err != nil {
			return xerrors.Errorf("couldn't init genesis block: %w", err)
		}
	}

	return nil
}

func (a skipchainActor) newChain(data proto.Message, conodes Conodes) error {
	randomBackLink := Digest{}
	n, err := a.rand.Read(randomBackLink[:])
	if err != nil {
		return xerrors.Errorf("couldn't generate backlink: %v", err)
	}
	if n != len(randomBackLink) {
		return xerrors.Errorf("mismatch rand length %d != %d", n, len(randomBackLink))
	}

	genesis, err := newSkipBlock(
		a.hashFactory,
		nil,
		0,
		conodes,
		Digest{},
		randomBackLink,
		data,
	)
	if err != nil {
		return xerrors.Errorf("couldn't create block: %v", err)
	}

	packed, err := genesis.Pack()
	if err != nil {
		return xerrors.Errorf("couldn't encode the block: %v", err)
	}

	msg := &PropagateGenesis{
		Genesis: packed.(*BlockProto),
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultPropogationTimeout)
	defer cancel()

	closing, errs := a.rpc.Call(ctx, msg, conodes)
	select {
	case <-closing:
		return nil
	case err := <-errs:
		return xerrors.Errorf("couldn't propagate: %v", err)
	}
}

// Store implements blockchain.Actor. It will append a new block to chain filled
// with the data.
func (a skipchainActor) Store(data proto.Message, players mino.Players) error {
	factory := a.GetBlockFactory().(blockFactory)

	ca, ok := players.(cosi.CollectiveAuthority)
	if !ok {
		return xerrors.Errorf("players must implement cosi.CollectiveAuthority")
	}

	previous, err := a.db.ReadLast()
	if err != nil {
		return xerrors.Errorf("couldn't read the latest block: %v", err)
	}

	block, err := factory.fromPrevious(previous, data)
	if err != nil {
		return xerrors.Errorf("couldn't create next block: %v", err)
	}

	block.Conodes = newConodes(ca)

	// Wait for the view change module green signal to go through the proposal.
	// If the leader has failed and this node has to take over, we use the
	// inherant property of CoSiPBFT to prove that 2f participants want the view
	// change.
	rotation, err := a.viewchange.Wait(block)
	if err == nil {
		// If the node is not the current leader and a rotation is necessary, it
		// will be done.
		block.Conodes = rotation.(Conodes)
	} else {
		a.logger.Debug().Msgf("%v refusing view change: %v", a, err)
		// Not authorized to propose a block as the leader is moving
		// forward so we drop the proposal. The upper layer is responsible to
		// try again until the leader includes the data.
		return nil
	}

	err = a.consensus.Propose(block, players)
	if err != nil {
		return xerrors.Errorf("couldn't propose the block: %v", err)
	}

	return nil
}

// skipchainObserver can be registered in the watcher to listen for incoming new
// blocks.
//
// - implements blockchain.Observer
type skipchainObserver struct {
	ch chan blockchain.Block
}

// NotifyCallback implements blockchain.Observer. It sends the event to the
// channel if the type is correct, otherwise it issues a warning.
func (o skipchainObserver) NotifyCallback(event interface{}) {
	block, ok := event.(SkipBlock)
	if !ok {
		fabric.Logger.Warn().Msgf("got invalid event '%T'", event)
		return
	}

	o.ch <- block
}
