package skipchain

import (
	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/blockchain"
	"golang.org/x/xerrors"
)

// Validator is the validator provided by the user of the skipchain module.
type Validator interface {
	Validate(b SkipBlock) error
}

type blockValidator struct {
	factory   *blockFactory
	db        Database
	validator Validator
	triage    *blockTriage
}

func newBlockValidator(f *blockFactory, v Validator, db Database, t *blockTriage) *blockValidator {
	return &blockValidator{
		factory:   f,
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

	return nil, xerrors.New("invalid message type")
}

func (b *blockValidator) verifyAndHashBlock(in *blockchain.Block) ([]byte, error) {
	var da ptypes.DynamicAny
	err := ptypes.UnmarshalAny(in.GetPayload(), &da)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode the payload: %v", err)
	}

	unpacked, err := b.factory.fromBlock(in)
	block := unpacked.(SkipBlock)

	err = b.validator.Validate(block)
	if err != nil {
		return nil, xerrors.Errorf("couldn't validate the payload: %v", err)
	}

	// Keep the block for the commit phase.
	err = b.triage.Add(block)
	if err != nil {
		return nil, xerrors.Errorf("couldn't add the block to the triage: %v", err)
	}

	fl := forwardLink{
		from: block.BackLinks[0],
		to:   block.hash,
	}

	hash, err := fl.computeHash()
	if err != nil {
		return nil, xerrors.Errorf("couldn't compute the hash of the forward link: %v", err)
	}

	return hash, nil
}

func (b *blockValidator) verifyAndHashForwardLink(in *ForwardLinkProto) ([]byte, error) {
	fl, err := b.factory.fromForwardLink(in)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode the forward link: %v", err)
	}

	forwardLink := fl.(forwardLink)

	// Verify that the triage has a block waiting to be committed and that the
	// forward link is matching correctly.
	err = b.triage.Verify(forwardLink)
	if err != nil {
		return nil, xerrors.Errorf("couldn't verify the block in triage: %v", err)
	}

	buffer, err := forwardLink.prepare.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal the prepare signature: %v", err)
	}

	return buffer, nil
}
