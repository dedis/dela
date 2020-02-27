package skipchain

import (
	"errors"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/m/blockchain"
	"go.dedis.ch/m/crypto"
)

// Validator is the validator provided by the user of the skipchain module.
type Validator interface {
	Validate(b SkipBlock) error
}

type blockValidator struct {
	factory   blockFactory
	db        Database
	validator Validator
	triage    *blockTriage
}

func newBlockValidator(verifier crypto.Verifier, v Validator, db Database, t *blockTriage) *blockValidator {
	return &blockValidator{
		factory:   newBlockFactory(verifier),
		db:        db,
		validator: v,
		triage:    t,
	}
}

func (b *blockValidator) Validate(msg proto.Message) ([]byte, error) {
	switch input := msg.(type) {
	case *blockchain.Block:
		return b.verifyAndHashBlock(input)
	case *ForwardLinkProto:
		return b.verifyAndHashForwardLink(input)
	}

	return nil, errors.New("invalid message type")
}

func (b *blockValidator) verifyAndHashBlock(in *blockchain.Block) ([]byte, error) {
	var da ptypes.DynamicAny
	err := ptypes.UnmarshalAny(in.GetPayload(), &da)
	if err != nil {
		return nil, err
	}

	unpacked, err := b.factory.fromBlock(in)
	block := unpacked.(SkipBlock)

	err = b.validator.Validate(block)
	if err != nil {
		return nil, err
	}

	// Keep the block for the commit phase.
	err = b.triage.Add(block)
	if err != nil {
		return nil, err
	}

	fl := ForwardLink{
		From: block.BackLinks[0],
		To:   block.hash,
	}

	hash, err := fl.computeHash()
	if err != nil {
		return nil, err
	}

	return hash, nil
}

func (b *blockValidator) verifyAndHashForwardLink(in *ForwardLinkProto) ([]byte, error) {
	fl, err := b.factory.fromForwardLink(in)
	if err != nil {
		return nil, err
	}

	// Verify that the triage has a block waiting to be committed and that the
	// forward link is matching correctly.
	err = b.triage.Verify(fl)
	if err != nil {
		return nil, err
	}

	buffer, err := fl.Prepare.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return buffer, nil
}
