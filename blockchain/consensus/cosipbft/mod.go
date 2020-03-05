package cosipbft

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/blockchain/consensus"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/cosi/blscosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Consensus implements the Consensus interface using Collective Signing PBFT
// and forward links as seals.
type Consensus struct {
	factory *sealFactory
	cosi    cosi.CollectiveSigning
}

// NewConsensus returns a new consensus instance.
func NewConsensus(o mino.Mino, signer crypto.AggregateSigner, t Triage) (Consensus, error) {
	factory := &sealFactory{verifier: signer}

	validator := validator{
		triage:  t,
		factory: factory,
	}

	cosi, err := blscosi.NewBlsCoSi(o, signer, validator)
	if err != nil {
		return Consensus{}, err
	}

	return Consensus{cosi: cosi, factory: factory}, nil
}

// GetSealFactory returns an instance of a seal factory.
func (c Consensus) GetSealFactory() consensus.SealFactory {
	return c.factory
}

// Propose implements the PBFT over Collective Signing protocol.
func (c Consensus) Propose(roster blockchain.Roster, from, proposal blockchain.Block) (consensus.Seal, error) {
	pt, err := proposal.Pack()
	if err != nil {
		return nil, err
	}

	pta, err := ptypes.MarshalAny(pt)
	if err != nil {
		return nil, err
	}

	req := &PrepareSignatureRequest{
		From:     from.GetID().Bytes(),
		Proposal: pta,
	}

	// 1. Prepare phase
	// The block is sent for validation and participants will return a signature
	// to confirm they agree the proposal is valid.
	// Signature = Sign(FROM_HASH + TO_HASH)
	sig, err := c.cosi.Sign(roster, req)
	if err != nil {
		return nil, xerrors.Errorf("couldn't sign the block: %v", err)
	}

	forwardLink := forwardLink{
		from:    from.GetID(),
		to:      proposal.GetID(),
		prepare: sig,
	}

	flproto, err := forwardLink.Pack()
	if err != nil {
		return nil, err
	}

	// 2. Commit phase
	// Forward link is updated with the prepare phase signature and then
	// participants will sign their commitment to the block after verifying
	// that the prepare signature is correct.
	// Signature = Sign(PREPARE_SIG)
	sig, err = c.cosi.Sign(roster, flproto)
	if err != nil {
		return nil, xerrors.Errorf("couldn't sign the forward link: %v", err)
	}

	forwardLink.commit = sig

	return forwardLink, nil
}

// Triage is an interface that must be implemented by the caller of CoSiPBFT so
// that the PREPARE and COMMIT phases can be validated.
type Triage interface {
	// Prepare proposes a block to the triage to be added as the future next
	// block.
	Prepare(block proto.Message) (blockchain.Block, error)

	// Commit should return an error if the proposed block cannot be committed,
	// and otherwise, it should lock the triage for it.
	Commit(proposed blockchain.BlockID) error
}

type validator struct {
	triage  Triage
	factory consensus.SealFactory
}

func (v validator) Validate(msg proto.Message) ([]byte, error) {
	switch input := msg.(type) {
	case *PrepareSignatureRequest:
		var block ptypes.DynamicAny
		err := ptypes.UnmarshalAny(input.GetProposal(), &block)
		if err != nil {
			return nil, err
		}

		from := blockchain.BlockID{}
		copy(from[:], input.GetFrom())

		return v.verifyAndHashBlock(from, block.Message)
	case *ForwardLinkProto:
		return v.verifyAndHashForwardLink(input)
	default:
		return nil, xerrors.New("invalid type of message")
	}
}

func (v validator) verifyAndHashBlock(from blockchain.BlockID, proposal proto.Message) ([]byte, error) {
	block, err := v.triage.Prepare(proposal)
	if err != nil {
		return nil, xerrors.Errorf("couldn't add the block to the triage: %v", err)
	}

	fl := forwardLink{
		from: from,
		to:   block.GetID(),
	}

	hash, err := fl.computeHash()
	if err != nil {
		return nil, xerrors.Errorf("couldn't compute the hash of the forward link: %v", err)
	}

	return hash, nil
}

func (v validator) verifyAndHashForwardLink(in proto.Message) ([]byte, error) {
	seal, err := v.factory.FromProto(in)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode the forward link: %v", err)
	}

	// Verify that the triage has a block waiting to be committed and that the
	// forward link is matching correctly.
	err = v.triage.Commit(seal.(forwardLink).GetTo())
	if err != nil {
		return nil, xerrors.Errorf("couldn't verify the block in triage: %v", err)
	}

	buffer, err := seal.(forwardLink).prepare.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal the prepare signature: %v", err)
	}

	return buffer, nil
}
