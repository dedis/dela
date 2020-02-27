package skipchain

import (
	"context"
	fmt "fmt"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/m"
	"go.dedis.ch/m/blockchain"
	"go.dedis.ch/m/cosi"
	"go.dedis.ch/m/cosi/blscosi"
	"go.dedis.ch/m/crypto"
	"go.dedis.ch/m/mino"
)

//go:generate protoc -I ./ --proto_path=../../ --go_out=Mblockchain/messages.proto=go.dedis.ch/m/blockchain:. ./messages.proto

// Skipchain implements the Blockchain interface by using collective signatures
// to create a verifiable chain.
type Skipchain struct {
	blockFactory blockFactory
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
	validator := newBlockValidator(signer, v, db, triage)

	cosi, err := blscosi.NewBlsCoSi(m, signer, validator)
	if err != nil {
		return nil, err
	}

	rpc, err := m.MakeRPC("skipchain", newHandler(db, triage, factory))
	if err != nil {
		return nil, err
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
	genesis := s.blockFactory.createGenesis(roster)

	packed, err := genesis.Pack()
	if err != nil {
		return err
	}

	msg := &PropagateGenesis{
		Genesis: packed.(*blockchain.Block),
	}

	closing, errs := s.rpc.Call(msg, roster.GetAddresses()...)
	select {
	case <-closing:
	case err := <-errs:
		m.Logger.Err(err).Msg("couldn't propagate genesis block")
		return err
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
		return err
	}

	block, err := s.blockFactory.FromPrevious(previous, data)
	if err != nil {
		return err
	}

	fl, err := s.pbft(block.(SkipBlock))
	if err != nil {
		return err
	}

	packed, err := fl.Pack()
	if err != nil {
		return err
	}

	msg := &PropagateForwardLink{
		Link: packed.(*ForwardLinkProto),
	}

	m.Logger.Info().Msgf("Propagation new block to %v", roster.GetAddresses())
	closing, errs := s.rpc.Call(msg, roster.GetAddresses()...)
	select {
	case <-closing:
	case err := <-errs:
		return fmt.Errorf("failed to propagate forward link: %v", err)
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
		return nil, err
	}

	sig, err := s.cosi.Sign(block.Roster, blockproto)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	sig, err = s.cosi.Sign(block.Roster, packed)
	if err != nil {
		return nil, err
	}

	fl.Commit = sig

	return fl, nil
}

// GetBlock returns the latest block.
func (s *Skipchain) GetBlock() (*blockchain.Block, error) {
	return nil, nil
}

// GetVerifiableBlock reads the latest block of the chain and creates a verifiable
// proof of the shortest chain from the genesis to the block.
func (s *Skipchain) GetVerifiableBlock() (*blockchain.VerifiableBlock, error) {
	return nil, nil
}

// Watch registers the observer so that it will be notified of new blocks.
func (s *Skipchain) Watch(ctx context.Context, obs blockchain.Observer) {

}
