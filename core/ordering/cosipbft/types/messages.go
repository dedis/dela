package types

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
)

var msgFormats = registry.NewSimpleRegistry()

func RegisterMessageFormat(f serde.Format, e serde.FormatEngine) {
	msgFormats.Register(f, e)
}

type GenesisMessage struct {
	genesis *Genesis
}

func NewGenesisMessage(genesis Genesis) GenesisMessage {
	return GenesisMessage{
		genesis: &genesis,
	}
}

func (m GenesisMessage) GetGenesis() *Genesis {
	return m.genesis
}

func (m GenesisMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// BlockMessage is a message sent to participants to share a block.
type BlockMessage struct{}

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
	signature crypto.Signature
}

func NewCommit(sig crypto.Signature) CommitMessage {
	return CommitMessage{
		signature: sig,
	}
}

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
	signature crypto.Signature
}

func NewDone(sig crypto.Signature) DoneMessage {
	return DoneMessage{
		signature: sig,
	}
}

func (m DoneMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type GenesisKey struct{}

type MessageFactory struct {
	genesisFac serde.Factory
}

func NewMessageFactory(gf serde.Factory) MessageFactory {
	return MessageFactory{
		genesisFac: gf,
	}
}

func (f MessageFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := msgFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, GenesisKey{}, f.genesisFac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
