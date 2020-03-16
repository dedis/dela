// Package skipchain implements the blockchain interface by using the skipchain
// design, e.i. blocks are linked by one or several forward links collectively
// signed by the participants.
//
// TODO: think about versioning for upgradability.
package skipchain

import (
	"context"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/consensus/cosipbft"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Skipchain implements the Blockchain interface by using collective signatures
// to create a verifiable chain.
type Skipchain struct {
	mino      mino.Mino
	cosi      cosi.CollectiveSigning
	db        Database
	consensus consensus.Consensus
}

// NewSkipchain returns a new instance of Skipchain.
func NewSkipchain(m mino.Mino, cosi cosi.CollectiveSigning) *Skipchain {
	consensus := cosipbft.NewCoSiPBFT(m, cosi)
	db := NewInMemoryDatabase()

	return &Skipchain{
		mino:      m,
		db:        db,
		consensus: consensus,
	}
}

// Listen starts the RPCs.
func (s *Skipchain) Listen(v blockchain.Validator) (blockchain.Actor, error) {
	actor := skipchainActor{
		Skipchain: s,
	}

	var err error
	actor.rpc, err = s.mino.MakeRPC("skipchain", newHandler(s))
	if err != nil {
		return nil, xerrors.Errorf("couldn't create the rpc: %v", err)
	}

	actor.consensus, err = s.consensus.Listen(&blockValidator{Skipchain: s, validator: v})
	if err != nil {
		return nil, xerrors.Errorf("couldn't start the consensus: %v", err)
	}

	return actor, nil
}

// GetBlockFactory returns the block factory for skipchains.
func (s *Skipchain) GetBlockFactory() (blockchain.BlockFactory, error) {
	return &blockFactory{}, nil
}

// GetBlock returns the latest block.
func (s *Skipchain) GetBlock() (blockchain.Block, error) {
	block, err := s.db.ReadLast()
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the latest block: %v", err)
	}

	return block, nil
}

// GetVerifiableBlock reads the latest block of the chain and creates a verifiable
// proof of the shortest chain from the genesis to the block.
func (s *Skipchain) GetVerifiableBlock() (blockchain.VerifiableBlock, error) {
	blocks, err := s.db.ReadAll()
	if err != nil {
		return nil, xerrors.Errorf("error when reading chain: %v", err)
	}

	if len(blocks) == 0 {
		return nil, xerrors.Errorf("expecting at least one block")
	}

	last := blocks[len(blocks)-1]

	chain, err := s.consensus.GetChain(last.GetHash())
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the chain: %v", err)
	}

	vb := VerifiableBlock{
		SkipBlock: last,
		Chain:     chain,
	}

	return vb, nil
}

// Watch registers the observer so that it will be notified of new blocks.
func (s *Skipchain) Watch(ctx context.Context, obs blockchain.Observer) {

}

type skipchainActor struct {
	*Skipchain
	consensus consensus.Actor
	rpc       mino.RPC
}

func (a skipchainActor) InitChain(data proto.Message, players mino.Players) error {
	factory := a.GetBlockFactory().(*blockFactory)
	if factory.genesis.GetHash() != nil {
		// Only allow one chain per database.
		return xerrors.Errorf("chain already exists: %v", factory.genesis)
	}

	ca, ok := players.(cosi.CollectiveAuthority)
	if !ok {
		return xerrors.Errorf("players must implement cosi.CollectiveAuthority")
	}

	conodes := newConodes(ca)

	genesis, err := factory.createGenesis(conodes, nil)
	if err != nil {
		return xerrors.Errorf("couldn't create the genesis block: %v", err)
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
		fabric.Logger.Err(err).Msg("couldn't propagate genesis block")
		return xerrors.Errorf("error in propagation: %v", err)
	}

	return nil
}

// Store will append a new block to chain filled with the data.
func (a skipchainActor) Store(data proto.Message, players mino.Players) error {
	factory := a.GetBlockFactory().(*blockFactory)

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
