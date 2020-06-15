package byzcoin

import (
	"io"

	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/ledger/byzcoin/json"
	"go.dedis.ch/dela/ledger/transactions"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// Blueprint is a message to propose a list of transactions to create a new
// block.
//
// - implements serde.Message
type Blueprint struct {
	serde.UnimplementedMessage

	transactions []transactions.ServerTransaction
}

// VisitJSON implements serde.Message. It serializes the blueprint message in
// JSON format.
func (b Blueprint) VisitJSON(ser serde.Serializer) (interface{}, error) {
	txs := make(json.Transactions, len(b.transactions))
	for i, tx := range b.transactions {
		raw, err := ser.Serialize(tx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize tx: %v", err)
		}

		txs[i] = raw
	}

	m := json.Blueprint{
		Transactions: txs,
	}

	return json.Message{Blueprint: &m}, nil
}

// GenesisPayload is a message to define the payload of the very first block of
// the chain.
//
// - implements serde.Message
// - implements serde.Fingerprinter
type GenesisPayload struct {
	serde.UnimplementedMessage

	roster viewchange.Authority
	root   []byte
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

// VisitJSON implements serde.Message. It serializes the genesis payload in JSON
// format.
func (p GenesisPayload) VisitJSON(ser serde.Serializer) (interface{}, error) {
	roster, err := ser.Serialize(p.roster)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize roster: %v", err)
	}

	m := json.GenesisPayload{
		Roster: roster,
		Root:   p.root,
	}

	return json.Message{GenesisPayload: &m}, nil
}

// BlockPayload is a message to define the payload of the blocks after the
// genesis.
//
// - implements serde.Message
// - implements serde.Fingerprinter
type BlockPayload struct {
	serde.UnimplementedMessage

	transactions []transactions.ServerTransaction
	root         []byte
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

// VisitJSON implements serde.Message. It serializes the block payload in JSON
// format.
func (p BlockPayload) VisitJSON(ser serde.Serializer) (interface{}, error) {
	txs := make(json.Transactions, len(p.transactions))
	for i, tx := range p.transactions {
		raw, err := ser.Serialize(tx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize tx: %v", err)
		}

		txs[i] = raw
	}

	m := json.BlockPayload{
		Transactions: txs,
		Root:         p.root,
	}

	return json.Message{BlockPayload: &m}, nil
}

// MessageFactory is a message factory to deserialize the blueprint and payload
// messages.
//
// - implements serde.Factory
type MessageFactory struct {
	serde.UnimplementedFactory

	rosterFactory serde.Factory
	txFactory     serde.Factory
}

// VisitJSON implements serde.Factory. It deserializes the blueprint or payload
// messages in JSON format.
func (f MessageFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Message{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	if m.GenesisPayload != nil {
		var roster viewchange.Authority
		err = in.GetSerializer().Deserialize(m.GenesisPayload.Roster, f.rosterFactory, &roster)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize roster: %v", err)
		}

		p := GenesisPayload{
			roster: roster,
			root:   m.GenesisPayload.Root,
		}

		return p, nil
	}
	if m.BlockPayload != nil {
		txs, err := f.txs(in.GetSerializer(), m.BlockPayload.Transactions)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize payload: %v", err)
		}

		p := BlockPayload{
			transactions: txs,
			root:         m.BlockPayload.Root,
		}

		return p, nil
	}
	if m.Blueprint != nil {
		txs, err := f.txs(in.GetSerializer(), m.Blueprint.Transactions)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize blueprint: %v", err)
		}

		b := Blueprint{
			transactions: txs,
		}

		return b, nil
	}

	return nil, xerrors.New("message is empty")
}

func (f MessageFactory) txs(ser serde.Serializer,
	raw json.Transactions) ([]transactions.ServerTransaction, error) {

	txs := make([]transactions.ServerTransaction, len(raw))
	for i, data := range raw {
		var tx transactions.ServerTransaction
		err := ser.Deserialize(data, f.txFactory, &tx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize tx: %v", err)
		}

		txs[i] = tx
	}

	return txs, nil
}
