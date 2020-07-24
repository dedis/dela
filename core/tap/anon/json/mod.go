package json

import (
	"go.dedis.ch/dela/core/tap/anon"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	anon.RegisterTransactionFormat(serde.FormatJSON, txFormat{})
}

// TransactionJSON is the JSON message of a transaction.
type TransactionJSON struct {
	Nonce uint64
	Args  map[string][]byte
}

// TxFormat is the JSON format engine for transactions.
//
// - implements serde.FormatEngine
type txFormat struct {
	hashFactory crypto.HashFactory
}

// Encode implements serde.FormatEngine. It returns the JSON data of the
// provided transaction if appropriate, otherwise it returns an error.
func (fmt txFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	tx, ok := msg.(anon.Transaction)
	if !ok {
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	args := map[string][]byte{}
	for _, arg := range tx.GetArgs() {
		args[arg] = tx.GetArg(arg)
	}

	m := TransactionJSON{
		Nonce: tx.GetNonce(),
		Args:  args,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It returns the transaction from the
// JSON data if appropriate, otherwise it returns an error.
func (fmt txFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := TransactionJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal: %v", err)
	}

	args := make([]anon.TransactionOption, 0, len(m.Args)+1)
	for key, value := range m.Args {
		args = append(args, anon.WithArg(key, value))
	}

	if fmt.hashFactory != nil {
		args = append(args, anon.WithHashFactory(fmt.hashFactory))
	}

	tx, err := anon.NewTransaction(m.Nonce, args...)
	if err != nil {
		return nil, xerrors.Errorf("failed to create tx: %v", err)
	}

	return tx, nil
}
