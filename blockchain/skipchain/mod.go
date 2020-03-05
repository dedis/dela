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
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/cosi/blscosi"
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
	cosi         cosi.CollectiveSigning
	rpc          mino.RPC
}

// NewSkipchain returns a new instance of Skipchain.
func NewSkipchain(m mino.Mino, signer crypto.AggregateSigner, v Validator) (*Skipchain, error) {
	db := NewInMemoryDatabase()
	factory := newBlockFactory(signer)
	triage := newBlockTriage(db, signer)
	validator := newBlockValidator(factory, v, db, triage)

	cosi, err := blscosi.NewBlsCoSi(m, signer, validator)
	if err != nil {
		return nil, xerrors.Errorf("couldn't create blscosi: %v", err)
	}

	rpc, err := m.MakeRPC("skipchain", newHandler(db, triage, factory))
	if err != nil {
		return nil, xerrors.Errorf("couldn't create the rpc: %v", err)
	}

	sc := &Skipchain{
		blockFactory: factory,
		db:           db,
		signer:       signer,
		cosi:         cosi,
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
		Genesis: packed.(*blockchain.Block),
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

	fl, err := s.pbft(block)
	if err != nil {
		return xerrors.Errorf("couldn't sign the block: %v", err)
	}

	packed, err := fl.Pack()
	if err != nil {
		return xerrors.Errorf("couldn't encode the forward link: %v", err)
	}

	msg := &PropagateForwardLink{
		Link: packed.(*ForwardLinkProto),
	}

	fabric.Logger.Info().Msgf("Propagation new block to %v", roster.GetAddresses())
	closing, errs := s.rpc.Call(msg, roster.GetAddresses()...)
	select {
	case <-closing:
	case err := <-errs:
		return xerrors.Errorf("failed to propagate forward link: %v", err)
	}

	return nil
}

func (s *Skipchain) pbft(block SkipBlock) (*ForwardLink, error) {
	// 1. Prepare phase
	// The block is sent for validation and participants will return a signature
	// to confirm they agree the proposal is valid.
	// Signature = Sign(FROM_HASH + TO_HASH)
	blockproto, err := block.Pack()
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode the block: %v", err)
	}

	sig, err := s.cosi.Sign(block.Roster, blockproto)
	if err != nil {
		return nil, xerrors.Errorf("couldn't sign the block: %v", err)
	}

	// 2. Commit phase
	// Forward link is updated with the prepare phase signature and then
	// participants will sign their commitment to the block after verifying
	// that the prepare signature is correct.
	// Signature = Sign(PREPARE_SIG)
	fl := &ForwardLink{
		From:    block.BackLinks[0],
		To:      block.hash,
		Prepare: sig,
	}

	packed, err := fl.Pack()
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode the forward link: %v", err)
	}

	sig, err = s.cosi.Sign(block.Roster, packed)
	if err != nil {
		return nil, xerrors.Errorf("couldn't sign the forward link: %v", err)
	}

	fl.Commit = sig

	return fl, nil
}

// GetBlock returns the latest block.
func (s *Skipchain) GetBlock() (*blockchain.Block, error) {
	block, err := s.db.ReadLast()
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the latest block: %v", err)
	}

	packed, err := block.Pack()
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode the block: %v", err)
	}

	return packed.(*blockchain.Block), nil
}

// GetVerifiableBlock reads the latest block of the chain and creates a verifiable
// proof of the shortest chain from the genesis to the block.
func (s *Skipchain) GetVerifiableBlock() (*blockchain.VerifiableBlock, error) {
	blocks, err := s.db.ReadChain()
	if err != nil {
		return nil, xerrors.Errorf("error when reading chain: %v", err)
	}

	// Encode the latest block to a protobuf message.
	packed, err := blocks[len(blocks)-1].Pack()
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack the block: %v", err)
	}

	// Get the chain of forward links from other blocks and encode
	// them into a protobuf proof.
	proof, err := blocks.GetProof().Pack()
	if err != nil {
		return nil, xerrors.Errorf("couldn't get the proof: %v", err)
	}

	proofAny, err := ptypes.MarshalAny(proof)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack the proof: %v", err)
	}

	vBlock := &blockchain.VerifiableBlock{
		Block: packed.(*blockchain.Block),
		Proof: proofAny,
	}

	return vBlock, nil
}

// Watch registers the observer so that it will be notified of new blocks.
func (s *Skipchain) Watch(ctx context.Context, obs blockchain.Observer) {

}
