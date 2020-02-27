package skipchain

import (
	"bytes"
	"crypto/sha256"
	"errors"
	fmt "fmt"

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
	queue     SkipBlock
	verifier  crypto.Verifier
}

func newBlockValidator(verifier crypto.Verifier, v Validator, db Database) *blockValidator {
	return &blockValidator{
		factory:   newBlockFactory(verifier),
		db:        db,
		validator: v,
		verifier:  verifier,
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

func (b *blockValidator) verifyAndHashBlock(block *blockchain.Block) ([]byte, error) {
	var da ptypes.DynamicAny
	err := ptypes.UnmarshalAny(block.GetPayload(), &da)
	if err != nil {
		return nil, err
	}

	unpacked, err := b.factory.fromBlock(block)
	sb := unpacked.(SkipBlock)

	err = b.validator.Validate(sb)
	if err != nil {
		return nil, err
	}

	last, err := b.db.ReadLast()
	if err != nil {
		return nil, err
	}

	if sb.Index <= last.Index {
		return nil, fmt.Errorf("wrong index: %d <= %d", block.Index, last.Index)
	}

	// Keep the block for the commit phase.
	b.queue = sb

	fl := ForwardLink{
		From: sb.BackLinks[0],
		To:   sb.hash,
	}

	err = fl.computeHash()
	if err != nil {
		return nil, err
	}

	return fl.hash, nil
}

func (b *blockValidator) verifyAndHashForwardLink(in *ForwardLinkProto) ([]byte, error) {
	if !bytes.Equal(in.To, b.queue.hash) {
		return nil, errors.New("forward link does not match queued block")
	}

	fl, err := b.factory.fromForwardLink(in)
	if err != nil {
		return nil, err
	}

	err = b.verifier.Verify(b.queue.Roster.GetPublicKeys(), fl.hash, fl.Prepare)
	if err != nil {
		return nil, err
	}

	buffer, err := fl.Prepare.MarshalBinary()
	if err != nil {
		return nil, err
	}

	h := sha256.New()
	h.Write(buffer)

	return h.Sum(nil), nil
}
