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

func RegisterMessageFormat(c serde.Codec, f serde.Format) {
	msgFormats.Register(c, f)
}

// Blueprint is a message to propose a list of transactions to create a new
// block.
//
// - implements serde.Message
type Blueprint struct {
	txs []transactions.ServerTransaction
}

func NewBlueprint(txs []transactions.ServerTransaction) Blueprint {
	return Blueprint{
		txs: txs,
	}
}

func (b Blueprint) GetTransactions() []transactions.ServerTransaction {
	return append([]transactions.ServerTransaction{}, b.txs...)
}

// Serialize implements serde.Message.
func (b Blueprint) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, b)
	if err != nil {
		return nil, err
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

func NewGenesisPayload(root []byte, roster viewchange.Authority) GenesisPayload {
	return GenesisPayload{
		root:   root,
		roster: roster,
	}
}

func (p GenesisPayload) GetRoot() []byte {
	return append([]byte{}, p.root...)
}

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

// Serialize implements serde.Message.
func (p GenesisPayload) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, p)
	if err != nil {
		return nil, err
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

func NewBlockPayload(root []byte, txs []transactions.ServerTransaction) BlockPayload {
	return BlockPayload{
		root: root,
		txs:  txs,
	}
}

func (p BlockPayload) GetRoot() []byte {
	return append([]byte{}, p.root...)
}

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
	format := msgFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, p)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type RosterKey struct{}
type TxKey struct{}

// MessageFactory is a message factory to deserialize the blueprint and payload
// messages.
//
// - implements serde.Factory
type MessageFactory struct {
	rosterFactory viewchange.AuthorityFactory
	txFactory     transactions.TxFactory
}

func NewMessageFactory(rf viewchange.AuthorityFactory, tf transactions.TxFactory) MessageFactory {
	return MessageFactory{
		rosterFactory: rf,
		txFactory:     tf,
	}
}

// Deserialize implements serde.Factory.
func (f MessageFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := msgFormats.Get(ctx.GetName())

	ctx = serde.WithFactory(ctx, RosterKey{}, f.rosterFactory)
	ctx = serde.WithFactory(ctx, TxKey{}, f.txFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
