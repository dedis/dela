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

//go:generate protoc -I ./ --proto_path=../../ --go_out=Mblockchain/messages.proto=go.dedis.ch/fabric/blockchain:. ./messages.proto

// Skipchain implements the Blockchain interface by using collective signatures
// to create a verifiable chain.
type Skipchain struct {
	mino         mino.Mino
	blockFactory *blockFactory
	db           Database
	consensus    consensus.Consensus
	rpc          mino.RPC
}

// NewSkipchain returns a new instance of Skipchain.
func NewSkipchain(m mino.Mino, cosi cosi.CollectiveSigning) *Skipchain {
	consensus := cosipbft.NewCoSiPBFT(m, cosi)

	return &Skipchain{
		mino: m,
		blockFactory: newBlockFactory(
			cosi.GetVerifier(),
			consensus.GetChainFactory(),
			m.GetAddressFactory(),
		),
		db:        NewInMemoryDatabase(),
		consensus: consensus,
	}
}

func (s *Skipchain) initChain(conodes Conodes) error {
	genesis, err := s.blockFactory.createGenesis(conodes, nil)
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

	closing, errs := s.rpc.Call(msg, conodes.GetNodes()...)
	select {
	case <-closing:
	case err := <-errs:
		fabric.Logger.Err(err).Msg("couldn't propagate genesis block")
		return xerrors.Errorf("error in propagation: %v", err)
	}

	return nil
}

// Listen starts the RPCs.
func (s *Skipchain) Listen(v blockchain.Validator) error {
	rpc, err := s.mino.MakeRPC("skipchain", newHandler(s.db, s.blockFactory))
	if err != nil {
		return xerrors.Errorf("couldn't create the rpc: %v", err)
	}

	s.rpc = rpc

	err = s.consensus.Listen(&blockValidator{Skipchain: s, validator: v})
	if err != nil {
		return xerrors.Errorf("couldn't start the consensus: %v", err)
	}

	return nil
}

// GetBlockFactory returns the block factory for skipchains.
func (s *Skipchain) GetBlockFactory() blockchain.BlockFactory {
	return s.blockFactory
}

// Store will append a new block to chain filled with the data.
func (s *Skipchain) Store(data proto.Message, nodes ...mino.Node) error {
	previous, err := s.db.ReadLast()
	if err != nil {
		return xerrors.Errorf("couldn't read the latest block: %v", err)
	}

	block, err := s.blockFactory.fromPrevious(previous, data)
	if err != nil {
		return xerrors.Errorf("couldn't create next block: %v", err)
	}

	err = s.consensus.Propose(block, nodes...)
	if err != nil {
		return xerrors.Errorf("couldn't propose the block: %v", err)
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

// GetVerifiableBlock reads the latest block of the chain and creates a verifiable
// proof of the shortest chain from the genesis to the block.
func (s *Skipchain) GetVerifiableBlock() (blockchain.VerifiableBlock, error) {
	blocks, err := s.db.ReadAll()
	if err != nil {
		return nil, xerrors.Errorf("error when reading chain: %v", err)
	}

	chain := s.consensus.GetChain(0, 0)

	vb := VerifiableBlock{
		SkipBlock: blocks[len(blocks)-1],
		Chain:     chain,
	}

	return vb, nil
}

// Watch registers the observer so that it will be notified of new blocks.
func (s *Skipchain) Watch(ctx context.Context, obs blockchain.Observer) {

}
