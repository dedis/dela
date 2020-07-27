package types

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
)

var msgFormats = registry.NewSimpleRegistry()

// RegisterMessageFormat registers the engine for the provided format.
func RegisterMessageFormat(f serde.Format, e serde.FormatEngine) {
	msgFormats.Register(f, e)
}

// GenesisMessage is a message to send a genesis to distant participants.
type GenesisMessage struct {
	genesis *Genesis
}

// NewGenesisMessage creates a new genesis message.
func NewGenesisMessage(genesis Genesis) GenesisMessage {
	return GenesisMessage{
		genesis: &genesis,
	}
}

// GetGenesis returns the genesis block contained in the message.
func (m GenesisMessage) GetGenesis() *Genesis {
	return m.genesis
}

// Serialize implements serde.Message.
func (m GenesisMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// BlockMessage is a message sent to participants to share a block.
type BlockMessage struct {
	block Block
}

// NewBlockMessage creates a new block message with the provided block.
func NewBlockMessage(block Block) BlockMessage {
	return BlockMessage{
		block: block,
	}
}

// GetBlock returns the block of the message.
func (m BlockMessage) GetBlock() Block {
	return m.block
}

// Serialize implements serde.Message.
func (m BlockMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// CommitMessage is a message containing the signature of the prepare phase of a
// PBFT execution.
type CommitMessage struct {
	id        []byte
	signature crypto.Signature
}

// NewCommit creates a new commit message.
func NewCommit(id []byte, sig crypto.Signature) CommitMessage {
	return CommitMessage{
		id:        id,
		signature: sig,
	}
}

func (m CommitMessage) GetID() []byte {
	return append([]byte{}, m.id...)
}

func (m CommitMessage) GetSignature() crypto.Signature {
	return m.signature
}

// Serialize implements serde.Message.
func (m CommitMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// DoneMessage is a message containing the signature of the commit phase of a
// PBFT execution.
type DoneMessage struct {
	id        []byte
	signature crypto.Signature
}

// NewDone creates a new done message.
func NewDone(id []byte, sig crypto.Signature) DoneMessage {
	return DoneMessage{
		id:        id,
		signature: sig,
	}
}

func (m DoneMessage) GetID() []byte {
	return append([]byte{}, m.id...)
}

func (m DoneMessage) GetSignature() crypto.Signature {
	return m.signature
}

// Serialize implements serde.Message.
func (m DoneMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// GenesisKey is the key of the genesis factory.
type GenesisKey struct{}

// BlockKey is the key of the block factory.
type BlockKey struct{}

type SignatureKey struct{}

// MessageFactory is the factory to deserialize messages.
type MessageFactory struct {
	genesisFac serde.Factory
	blockFac   serde.Factory
	sigFac     crypto.SignatureFactory
}

// NewMessageFactory creates a new message factory.
func NewMessageFactory(gf, bf serde.Factory, sf crypto.SignatureFactory) MessageFactory {
	return MessageFactory{
		genesisFac: gf,
		blockFac:   bf,
		sigFac:     sf,
	}
}

// Deserialize implements serde.Factory.
func (f MessageFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := msgFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, GenesisKey{}, f.genesisFac)
	ctx = serde.WithFactory(ctx, BlockKey{}, f.blockFac)
	ctx = serde.WithFactory(ctx, SignatureKey{}, f.sigFac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
