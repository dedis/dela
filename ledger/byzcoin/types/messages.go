package types

import (
	"io"

	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/ledger/transactions"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var msgFormats = registry.NewSimpleRegistry()

// RegisterMessageFormat registers the engine for the provided format.
func RegisterMessageFormat(c serde.Format, f serde.FormatEngine) {
	msgFormats.Register(c, f)
}

// Blueprint is a message to propose a list of transactions to create a new
// block.
//
// - implements serde.Message
type Blueprint struct {
	txs []transactions.ServerTransaction
}

// NewBlueprint creates a new blueprint message from the list of transactions.
func NewBlueprint(txs []transactions.ServerTransaction) Blueprint {
	return Blueprint{
		txs: txs,
	}
}

// GetTransactions returns the list of transactions for the blueprint.
func (b Blueprint) GetTransactions() []transactions.ServerTransaction {
	return append([]transactions.ServerTransaction{}, b.txs...)
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the blueprint.
func (b Blueprint) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, b)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode blueprint: %v", err)
	}

	return data, nil
}

// GenesisPayload is a message to define the payload of the very first block of
// the chain.
//
// - implements serde.Message
// - implements serde.Fingerprinter
type GenesisPayload struct {
	roster viewchange.Authority
	root   []byte
}

// NewGenesisPayload creates a new payload from the root and the roster.
func NewGenesisPayload(root []byte, roster viewchange.Authority) GenesisPayload {
	return GenesisPayload{
		root:   root,
		roster: roster,
	}
}

// GetRoot returns the root that is associated with an inventory page.
func (p GenesisPayload) GetRoot() []byte {
	return append([]byte{}, p.root...)
}

// GetRoster returns the initial roster of a chain.
func (p GenesisPayload) GetRoster() viewchange.Authority {
	return p.roster
}

// Fingerprint implements serde.Fingerprinter. It writes a deterministic binary
// representation of the payload to the writer.
func (p GenesisPayload) Fingerprint(w io.Writer) error {
	_, err := w.Write(p.root)
	if err != nil {
		return xerrors.Errorf("couldn't write root: %v", err)
	}

	// The integrity of the roster is ensured by the root. The roster in the
	// payload only serves as a replayable input but it should be read from the
	// inventory with a proof for a client request.

	return nil
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data for the payload.
func (p GenesisPayload) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, p)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode payload: %v", err)
	}

	return data, nil
}

// BlockPayload is a message to define the payload of the blocks after the
// genesis.
//
// - implements serde.Message
// - implements serde.Fingerprinter
type BlockPayload struct {
	txs  []transactions.ServerTransaction
	root []byte
}

// NewBlockPayload returns a new block payload from the root and the list of
// transactions.
func NewBlockPayload(root []byte, txs []transactions.ServerTransaction) BlockPayload {
	return BlockPayload{
		root: root,
		txs:  txs,
	}
}

// GetRoot returns the root associated with an inventory page.
func (p BlockPayload) GetRoot() []byte {
	return append([]byte{}, p.root...)
}

// GetTransactions return the list of transactions for the block payload.
func (p BlockPayload) GetTransactions() []transactions.ServerTransaction {
	return append([]transactions.ServerTransaction{}, p.txs...)
}

// Fingerprint implements serde.Fingerprinter. It writes a deterministic binary
// representation of the payload.
func (p BlockPayload) Fingerprint(w io.Writer) error {
	_, err := w.Write(p.root)
	if err != nil {
		return xerrors.Errorf("couldn't write root: %v", err)
	}

	// TODO: tx results

	return nil
}

// Serialize implements serde.Message.
func (p BlockPayload) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, p)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode payload: %v", err)
	}

	return data, nil
}

// RosterKey is the key of a roster factory.
type RosterKey struct{}

// TxKey is the key of a transaction factory.
type TxKey struct{}

// MessageFactory is a message factory to deserialize the blueprint and payload
// messages.
//
// - implements serde.Factory
type MessageFactory struct {
	rosterFactory viewchange.AuthorityFactory
	txFactory     transactions.TxFactory
}

// NewMessageFactory returns a new message factory.
func NewMessageFactory(rf viewchange.AuthorityFactory, tf transactions.TxFactory) MessageFactory {
	return MessageFactory{
		rosterFactory: rf,
		txFactory:     tf,
	}
}

// Deserialize implements serde.Factory.
func (f MessageFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := msgFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, RosterKey{}, f.rosterFactory)
	ctx = serde.WithFactory(ctx, TxKey{}, f.txFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode message: %v", err)
	}

	return msg, nil
}
