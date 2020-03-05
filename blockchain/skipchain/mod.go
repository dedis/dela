// Package skipchain implements the blockchain interface by using the skipchain
// design, e.i. blocks are linked by one or several forward links collectively
// signed by the participants.
//
// TODO: think about versioning for upgradability.
package skipchain

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/blockchain/consensus"
	"go.dedis.ch/fabric/blockchain/consensus/cosipbft"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --proto_path=../../ --go_out=Mblockchain/messages.proto=go.dedis.ch/fabric/blockchain:. ./messages.proto

// Skipchain implements the Blockchain interface by using collective signatures
// to create a verifiable chain.
type Skipchain struct {
	blockFactory *blockFactory
	db           Database
	signer       crypto.Signer
	consensus    consensus.Consensus
	rpc          mino.RPC
}

// NewSkipchain returns a new instance of Skipchain.
func NewSkipchain(m mino.Mino, signer crypto.AggregateSigner, v PayloadValidator) (*Skipchain, error) {
	db := NewInMemoryDatabase()
	factory := newBlockFactory(signer)
	triage := newBlockTriage(db, factory, v)

	consensus, err := cosipbft.NewConsensus(m, signer, triage)
	if err != nil {
		return nil, xerrors.Errorf("couldn't create consensus: %v", err)
	}

	rpc, err := m.MakeRPC("skipchain", newHandler(db, triage, factory))
	if err != nil {
		return nil, xerrors.Errorf("couldn't create the rpc: %v", err)
	}

	sc := &Skipchain{
		blockFactory: factory,
		db:           db,
		signer:       signer,
		consensus:    consensus,
		rpc:          rpc,
	}

	return sc, nil
}

func (s *Skipchain) initChain(roster blockchain.Roster) error {
	genesis, err := s.blockFactory.createGenesis(roster, nil)
	if err != nil {
		return xerrors.Errorf("couldn't create the genesis block: %v", err)
	}

	packed, err := genesis.Pack()
	if err != nil {
		return xerrors.Errorf("couldn't encode the block: %v", err)
	}

	msg := &PropagateGenesis{
		Genesis: packed.(*BlockProto),
	}

	closing, errs := s.rpc.Call(msg, roster.GetAddresses()...)
	select {
	case <-closing:
	case err := <-errs:
		fabric.Logger.Err(err).Msg("couldn't propagate genesis block")
		return xerrors.Errorf("error in propagation: %v", err)
	}

	return nil
}

// GetBlockFactory returns the block factory for skipchains.
func (s *Skipchain) GetBlockFactory() blockchain.BlockFactory {
	return s.blockFactory
}

// Store will append a new block to chain filled with the data.
func (s *Skipchain) Store(roster blockchain.Roster, data proto.Message) error {
	previous, err := s.db.ReadLast()
	if err != nil {
		return xerrors.Errorf("couldn't read the latest block: %v", err)
	}

	block, err := s.blockFactory.fromPrevious(previous, data)
	if err != nil {
		return xerrors.Errorf("couldn't create next block: %v", err)
	}

	seal, err := s.consensus.Propose(roster, previous, block)
	if err != nil {
		return xerrors.Errorf("couldn't propose the block: %v", err)
	}

	sealproto, err := seal.Pack()
	if err != nil {
		return xerrors.Errorf("couldn't encode the forward link: %v", err)
	}

	fabric.Logger.Info().Msgf("Propagation new block to %v", roster.GetAddresses())

	msg := &PropagateSeal{}
	msg.Seal, err = ptypes.MarshalAny(sealproto)
	if err != nil {
		return err
	}

	closing, errs := s.rpc.Call(msg, roster.GetAddresses()...)
	select {
	case <-closing:
	case err := <-errs:
		return xerrors.Errorf("failed to propagate forward link: %v", err)
	}

	return nil
}

// GetBlock returns the latest block.
func (s *Skipchain) GetBlock() (blockchain.Block, error) {
	block, err := s.db.ReadLast()
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the latest block: %v", err)
	}

	return block, nil
}

// GetVerifiableChain reads the latest block of the chain and creates a verifiable
// proof of the shortest chain from the genesis to the block.
func (s *Skipchain) GetVerifiableChain() (blockchain.Chain, error) {
	blocks, err := s.db.ReadChain()
	if err != nil {
		return nil, xerrors.Errorf("error when reading chain: %v", err)
	}

	return blocks.GetProof(), nil
}

// Watch registers the observer so that it will be notified of new blocks.
func (s *Skipchain) Watch(ctx context.Context, obs blockchain.Observer) {

}
