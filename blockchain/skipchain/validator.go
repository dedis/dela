package skipchain

import (
	"bytes"
	"crypto/sha256"
	"errors"
	fmt "fmt"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/m/blockchain"
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
}

func newBlockValidator(v Validator, db Database) *blockValidator {
	return &blockValidator{
		factory:   blockFactory{},
		db:        db,
		validator: v,
	}
}

func (b *blockValidator) Validate(msg proto.Message) ([]byte, error) {
	switch input := msg.(type) {
	case *blockchain.Block:
		return b.verifyAndHashBlock(input)
	case *ForwardLink:
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

	return sb.hash, nil
}

func (b *blockValidator) verifyAndHashForwardLink(fl *ForwardLink) ([]byte, error) {
	if !bytes.Equal(fl.To, b.queue.hash) {
		return nil, errors.New("forward link does not match queued block")
	}

	// TODO: verify the signature

	var dyn ptypes.DynamicAny
	err := ptypes.UnmarshalAny(fl.GetPrepare(), &dyn)
	if err != nil {
		return nil, err
	}

	buffer, err := proto.Marshal(dyn.Message)

	h := sha256.New()
	h.Write(buffer)

	return h.Sum(nil), nil
}
