package json

import (
	"encoding/json"

	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/ledger/byzcoin/types"
	"go.dedis.ch/dela/ledger/transactions"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterMessageFormat(serdeng.CodecJSON, messageFormat{})
}

// GenesisPayload is the JSON message of a genesis payload.
type GenesisPayload struct {
	Roster json.RawMessage
	Root   []byte
}

// Transactions is the JSON message for a list of transactions.
type Transactions []json.RawMessage

// BlockPayload is the JSON message for a block payload.
type BlockPayload struct {
	Transactions Transactions
	Root         []byte
}

// Blueprint is the JSON message for a new block proposal.
type Blueprint struct {
	Transactions Transactions
}

// Message is a JSON message container to deserialize any of the types.
type Message struct {
	Blueprint      *Blueprint      `json:",omitempty"`
	GenesisPayload *GenesisPayload `json:",omitempty"`
	BlockPayload   *BlockPayload   `json:",omitempty"`
}

type messageFormat struct{}

func (f messageFormat) Encode(ctx serdeng.Context, msg serdeng.Message) ([]byte, error) {
	var m Message

	switch in := msg.(type) {
	case types.Blueprint:
		txs := make(Transactions, len(in.GetTransactions()))
		for i, tx := range in.GetTransactions() {
			raw, err := tx.Serialize(ctx)
			if err != nil {
				return nil, xerrors.Errorf("couldn't serialize tx: %v", err)
			}

			txs[i] = raw
		}

		bp := Blueprint{
			Transactions: txs,
		}

		m = Message{Blueprint: &bp}
	case types.GenesisPayload:
		roster, err := in.GetRoster().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize roster: %v", err)
		}

		p := GenesisPayload{
			Roster: roster,
			Root:   in.GetRoot(),
		}

		m = Message{GenesisPayload: &p}
	case types.BlockPayload:
		txs := make(Transactions, len(in.GetTransactions()))
		for i, tx := range in.GetTransactions() {
			raw, err := tx.Serialize(ctx)
			if err != nil {
				return nil, xerrors.Errorf("couldn't serialize tx: %v", err)
			}

			txs[i] = raw
		}

		p := BlockPayload{
			Transactions: txs,
			Root:         in.GetRoot(),
		}

		m = Message{BlockPayload: &p}
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (f messageFormat) Decode(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	m := Message{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	if m.GenesisPayload != nil {
		roster, err := decodeRoster(ctx, m.GenesisPayload.Roster)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize roster: %v", err)
		}

		p := types.NewGenesisPayload(m.GenesisPayload.Root, roster)

		return p, nil
	}
	if m.BlockPayload != nil {
		txs, err := decodeTxs(ctx, m.BlockPayload.Transactions)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize payload: %v", err)
		}

		p := types.NewBlockPayload(m.BlockPayload.Root, txs)

		return p, nil
	}
	if m.Blueprint != nil {
		txs, err := decodeTxs(ctx, m.Blueprint.Transactions)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize blueprint: %v", err)
		}

		b := types.NewBlueprint(txs)

		return b, nil
	}

	return nil, xerrors.New("message is empty")
}

func decodeRoster(ctx serdeng.Context, data []byte) (viewchange.Authority, error) {
	factory := ctx.GetFactory(types.RosterKey{})

	fac, ok := factory.(viewchange.AuthorityFactory)
	if !ok {
		return nil, xerrors.New("invalid factory")
	}

	authority, err := fac.AuthorityOf(ctx, data)
	if err != nil {
		return nil, err
	}

	return authority, nil
}

func decodeTxs(ctx serdeng.Context, raws Transactions) ([]transactions.ServerTransaction, error) {
	factory := ctx.GetFactory(types.TxKey{})

	fac, ok := factory.(transactions.TxFactory)
	if !ok {
		return nil, xerrors.New("invalid factory")
	}

	txs := make([]transactions.ServerTransaction, len(raws))
	for i, data := range raws {
		tx, err := fac.TxOf(ctx, data)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize tx: %v", err)
		}

		txs[i] = tx
	}

	return txs, nil
}
