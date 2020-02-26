package skipchain

import (
	"context"
	fmt "fmt"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/m"
	"go.dedis.ch/m/blockchain"
	"go.dedis.ch/m/cosi"
	"go.dedis.ch/m/cosi/blscosi"
	"go.dedis.ch/m/crypto"
	"go.dedis.ch/m/mino"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Skipchain implements the Blockchain interface by using collective signatures
// to create a verifiable chain.
type Skipchain struct {
	blockFactory blockFactory
	db           Database
	signer       crypto.Signer
	cosi         cosi.CollectiveSigning
}

// NewSkipchain returns a new instance of Skipchain.
func NewSkipchain(m mino.Mino, signer crypto.AggregateSigner, v Validator) (*Skipchain, error) {
	db := NewInMemoryDatabase()
	validator := newBlockValidator(v, db)

	cosi, err := blscosi.NewBlsCoSi(m, signer, validator)
	if err != nil {
		return nil, err
	}

	sc := &Skipchain{
		blockFactory: blockFactory{},
		db:           db,
		cosi:         cosi,
	}

	return sc, nil
}

func (s *Skipchain) initChain(roster blockchain.Roster) error {
	genesis := s.blockFactory.createGenesis(roster)

	err := s.db.Write(genesis)
	if err != nil {
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

	blockproto, err := block.(SkipBlock).Pack()
	if err != nil {
		return err
	}

	fl, err := s.pbft(blockproto.(*blockchain.Block))
	if err != nil {
		return err
	}

	verifier := s.cosi.MakeVerifier()
	commitSig, err := verifier.GetSignatureFactory().FromAny(fl.GetCommit())
	if err != nil {
		return err
	}

	m.Logger.Info().Msgf("Forward Link Commit: %x", commitSig)

	return nil
}

func (s *Skipchain) pbft(block *blockchain.Block) (*ForwardLink, error) {
	// 1. Prepare phase
	// The block is sent for validation and participants will return a signature
	// to confirm they agree the proposal is valid.
	// Signature = Sign(FROM_HASH + TO_HASH)
	sig, err := s.cosi.Sign(block.GetRoster(), block)
	if err != nil {
		return nil, err
	}

	m.Logger.Info().Str("hex", fmt.Sprintf("%x", sig)).Msg("Signature")

	sigproto, err := sig.Pack()
	if err != nil {
		return nil, err
	}

	packed, err := ptypes.MarshalAny(sigproto)
	if err != nil {
		return nil, err
	}

	// 2. Commit phase
	// Forward link is updated with the prepare phase signature and then
	// participants will sign their commitment to the block after verifying
	// that the prepare signature is correct.
	// Signature = Sign(PREPARE_SIG)
	fl := &ForwardLink{
		From:    nil,
		To:      nil,
		Prepare: packed,
	}

	sig, err = s.cosi.Sign(block.GetRoster(), fl)
	if err != nil {
		return nil, err
	}

	sigproto, err = sig.Pack()
	if err != nil {
		return nil, err
	}

	packed, err = ptypes.MarshalAny(sigproto)
	if err != nil {
		return nil, err
	}

	fl.Commit = packed

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
