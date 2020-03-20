// Package skipchain implements the blockchain interface by using the skipchain
// design, e.i. blocks are linked by one or several forward links collectively
// signed by the participants.
//
// TODO: think about versioning for upgradability.
package skipchain

import (
	"context"
	"crypto/rand"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/consensus/cosipbft"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Skipchain is a blockchain that is using collective signatures to create links
// between the blocks.
//
// - implements blockchain.Blockchain
type Skipchain struct {
	mino      mino.Mino
	cosi      cosi.CollectiveSigning
	db        Database
	consensus consensus.Consensus
	watcher   blockchain.Observable
}

// NewSkipchain returns a new instance of Skipchain.
func NewSkipchain(m mino.Mino, cosi cosi.CollectiveSigning) *Skipchain {
	consensus := cosipbft.NewCoSiPBFT(m, cosi)
	db := NewInMemoryDatabase()

	return &Skipchain{
		mino:      m,
		cosi:      cosi,
		db:        db,
		consensus: consensus,
		watcher:   blockchain.NewWatcher(),
	}
}

// Listen implements blockchain.Blockchain. It registers the RPC and starts the
// consensus module.
func (s *Skipchain) Listen(v blockchain.Validator) (blockchain.Actor, error) {
	actor := skipchainActor{
		Skipchain:   s,
		hashFactory: sha256Factory{},
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

// skipchainActor provides the primitives of a blockchain actor.
//
// - implements blockchain.Actor
type skipchainActor struct {
	*Skipchain
	hashFactory crypto.HashFactory
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

	conodes := newConodes(ca)

	// TODO: crypto module for randomness
	randomBackLink := Digest{}
	rand.Read(randomBackLink[:])

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

	err = a.db.Write(genesis)
	if err != nil {
		return xerrors.Errorf("couldn't write genesis block: %v", err)
	}

	packed, err := genesis.Pack()
	if err != nil {
		return xerrors.Errorf("couldn't encode the block: %v", err)
	}

	msg := &PropagateGenesis{
		Genesis: packed.(*BlockProto),
	}

	closing, errs := a.rpc.Call(msg, conodes)
	select {
	case <-closing:
	case err := <-errs:
		return xerrors.Errorf("couldn't propagate: %v", err)
	}

	return nil
}

// Store implements blockchain.Actor. It will append a new block to chain filled
// with the data.
func (a skipchainActor) Store(data proto.Message, players mino.Players) error {
	factory := a.GetBlockFactory().(blockFactory)

	previous, err := a.db.ReadLast()
	if err != nil {
		return xerrors.Errorf("couldn't read the latest block: %v", err)
	}

	block, err := factory.fromPrevious(previous, data)
	if err != nil {
		return xerrors.Errorf("couldn't create next block: %v", err)
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
