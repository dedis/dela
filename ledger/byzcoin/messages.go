package byzcoin

import (
	"io"

	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/ledger/byzcoin/json"
	"go.dedis.ch/dela/ledger/transactions"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

type Blueprint struct {
	serde.UnimplementedMessage

	transactions []transactions.ServerTransaction
}

func (b Blueprint) VisitJSON(ser serde.Serializer) (interface{}, error) {
	txs := make(json.Transactions, len(b.transactions))
	for i, tx := range b.transactions {
		raw, err := ser.Serialize(tx)
		if err != nil {
			return nil, err
		}

		txs[i] = raw
	}

	m := json.Blueprint{
		Transactions: txs,
	}

	return json.Message{Blueprint: &m}, nil
}

type GenesisPayload struct {
	serde.UnimplementedMessage

	roster viewchange.Authority
	root   []byte
}

func (p GenesisPayload) Fingerprint(w io.Writer) error {
	w.Write(p.root)

	return nil
}

func (p GenesisPayload) VisitJSON(ser serde.Serializer) (interface{}, error) {
	roster, err := ser.Serialize(p.roster)
	if err != nil {
		return nil, err
	}

	m := json.GenesisPayload{
		Roster: roster,
		Root:   p.root,
	}

	return json.Message{GenesisPayload: &m}, nil
}

type BlockPayload struct {
	serde.UnimplementedMessage

	transactions []transactions.ServerTransaction
	root         []byte
}

func (p BlockPayload) Fingerprint(w io.Writer) error {
	w.Write(p.root)

	return nil
}

func (p BlockPayload) VisitJSON(ser serde.Serializer) (interface{}, error) {
	txs := make(json.Transactions, len(p.transactions))
	for i, tx := range p.transactions {
		raw, err := ser.Serialize(tx)
		if err != nil {
			return nil, err
		}

		txs[i] = raw
	}

	m := json.BlockPayload{
		Transactions: txs,
		Root:         p.root,
	}

	return json.Message{BlockPayload: &m}, nil
}

type MessageFactory struct {
	serde.UnimplementedFactory

	rosterFactory serde.Factory
	txFactory     serde.Factory
}

func (f MessageFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Message{}
	err := in.Feed(&m)
	if err != nil {
		return nil, err
	}

	if m.GenesisPayload != nil {
		var roster viewchange.Authority
		err = in.GetSerializer().Deserialize(m.GenesisPayload.Roster, f.rosterFactory, &roster)
		if err != nil {
			return nil, err
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
			return nil, err
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
			return nil, err
		}

		b := Blueprint{
			transactions: txs,
		}

		return b, nil
	}

	return nil, xerrors.New("message is empty")
}

func (f MessageFactory) txs(ser serde.Serializer, raw json.Transactions) ([]transactions.ServerTransaction, error) {
	txs := make([]transactions.ServerTransaction, len(raw))
	for i, data := range raw {
		var tx transactions.ServerTransaction
		err := ser.Deserialize(data, f.txFactory, &tx)
		if err != nil {
			return nil, err
		}

		txs[i] = tx
	}

	return txs, nil
}
