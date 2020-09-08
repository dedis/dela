package types

import (
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/common"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
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

// Serialize implements serde.Message. It returns the serialized data for this
// message.
func (m GenesisMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

// BlockMessage is a message sent to participants to share a block.
type BlockMessage struct {
	block Block
	// TODO: include view change messages if appropriate.
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
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

// CommitMessage is a message containing the signature of the prepare phase of a
// PBFT execution.
type CommitMessage struct {
	id        Digest
	signature crypto.Signature
}

// NewCommit creates a new commit message.
func NewCommit(id Digest, sig crypto.Signature) CommitMessage {
	return CommitMessage{
		id:        id,
		signature: sig,
	}
}

// GetID returns the block digest to commit.
func (m CommitMessage) GetID() Digest {
	return m.id
}

// GetSignature returns the prepare signature.
func (m CommitMessage) GetSignature() crypto.Signature {
	return m.signature
}

// Serialize implements serde.Message.
func (m CommitMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

// DoneMessage is a message containing the signature of the commit phase of a
// PBFT execution.
type DoneMessage struct {
	id        Digest
	signature crypto.Signature
}

// NewDone creates a new done message.
func NewDone(id Digest, sig crypto.Signature) DoneMessage {
	return DoneMessage{
		id:        id,
		signature: sig,
	}
}

// GetID returns the digest of the block that has been accepted.
func (m DoneMessage) GetID() Digest {
	return m.id
}

// GetSignature returns the commit signature that proves the commitment of the
// block.
func (m DoneMessage) GetSignature() crypto.Signature {
	return m.signature
}

// Serialize implements serde.Message.
func (m DoneMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

// ViewMessage is a message to announce a view change request.
type ViewMessage struct {
	id        Digest
	leader    uint16
	signature crypto.Signature
}

// NewViewMessage creates a new view message.
func NewViewMessage(id Digest, leader uint16, sig crypto.Signature) ViewMessage {
	return ViewMessage{
		id:        id,
		leader:    leader,
		signature: sig,
	}
}

// GetID returns the digest of the latest block.
func (m ViewMessage) GetID() Digest {
	return m.id
}

// GetLeader returns the leader index of the view change.
func (m ViewMessage) GetLeader() uint16 {
	return m.leader
}

// GetSignature returns the signature of the view.
func (m ViewMessage) GetSignature() crypto.Signature {
	return m.signature
}

// Serialize implements serde.Message. It returns the serialized data for this
// message.
func (m ViewMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

// GenesisKey is the key of the genesis factory.
type GenesisKey struct{}

// BlockKey is the key of the block factory.
type BlockKey struct{}

// LinkKey is the key of the link factory.
type LinkKey struct{}

// AggregateKey is the key of the collective signature factory.
type AggregateKey struct{}

// SignatureKey is the key of the view signature factory.
type SignatureKey struct{}

// MessageFactory is the factory to deserialize messages.
type MessageFactory struct {
	genesisFac serde.Factory
	blockFac   serde.Factory
	aggFac     crypto.SignatureFactory
	sigFac     crypto.SignatureFactory
	csFac      authority.ChangeSetFactory
}

// NewMessageFactory creates a new message factory.
func NewMessageFactory(gf, bf serde.Factory, aggFac crypto.SignatureFactory, csf authority.ChangeSetFactory) MessageFactory {
	return MessageFactory{
		genesisFac: gf,
		blockFac:   bf,
		aggFac:     aggFac,
		sigFac:     common.NewSignatureFactory(),
		csFac:      csf,
	}
}

// Deserialize implements serde.Factory.
func (f MessageFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := msgFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, GenesisKey{}, f.genesisFac)
	ctx = serde.WithFactory(ctx, BlockKey{}, f.blockFac)
	ctx = serde.WithFactory(ctx, AggregateKey{}, f.aggFac)
	ctx = serde.WithFactory(ctx, SignatureKey{}, f.sigFac)
	ctx = serde.WithFactory(ctx, LinkKey{}, NewLinkFactory(f.blockFac, f.aggFac, f.csFac))

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("decoding failed: %v", err)
	}

	return msg, nil
}
