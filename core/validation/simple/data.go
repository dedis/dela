package simple

import (
	"io"

	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var (
	resFormats  = registry.NewSimpleRegistry()
	dataFormats = registry.NewSimpleRegistry()
)

func RegisterResultFormat(f serde.Format, e serde.FormatEngine) {
	resFormats.Register(f, e)
}

func RegisterDataFormat(f serde.Format, e serde.FormatEngine) {
	dataFormats.Register(f, e)
}

// TransactionResult is the result of a transaction processing. It contains the
// transaction and its state of success.
//
// - implements validation.TransactionResult
type TransactionResult struct {
	tx       tap.Transaction
	accepted bool
	reason   string
}

// NewTransactionResult creates a new transaction result for the provided
// transaction.
func NewTransactionResult(tx tap.Transaction, accepted bool, reason string) TransactionResult {
	return TransactionResult{
		tx:       tx,
		accepted: true,
		reason:   "",
	}
}

// GetTransaction implements validation.TransactionResult. It returns the
// transaction associated to the result.
func (res TransactionResult) GetTransaction() tap.Transaction {
	return res.tx
}

// GetStatus implements validation.TransactionResult. It returns true if the
// transaction has been accepted, otherwise false with the reason.
func (res TransactionResult) GetStatus() (bool, string) {
	return res.accepted, res.reason
}

// Serialize implements serde.Message. It returns the transaction result
// serialized.
func (res TransactionResult) Serialize(ctx serde.Context) ([]byte, error) {
	format := resFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, res)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type TransactionKey struct{}

type ResultFactory struct {
	fac tap.TransactionFactory
}

func NewResultFactory(f tap.TransactionFactory) ResultFactory {
	return ResultFactory{
		fac: f,
	}
}

func (f ResultFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := resFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, TransactionKey{}, f.fac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

// Data is the validated data of a standard validation.
//
// - implements validation.Data
type Data struct {
	txs []TransactionResult
}

func NewData(results []TransactionResult) Data {
	return Data{
		txs: results,
	}
}

// GetTransactionResults implements validation.Data. It returns the results.
func (d Data) GetTransactionResults() []validation.TransactionResult {
	res := make([]validation.TransactionResult, len(d.txs))
	for i, r := range d.txs {
		res[i] = r
	}

	return res
}

// Fingerprint implements serde.Fingerprinter. It writes a deterministic binary
// representation of the validated data.
func (d Data) Fingerprint(w io.Writer) error {
	for _, res := range d.txs {
		err := res.tx.Fingerprint(w)
		if err != nil {
			return xerrors.Errorf("couldn't fingerprint tx: %v", err)
		}

		bit := []byte{0}
		if res.accepted {
			bit[0] = 1
		}

		_, err = w.Write(bit)
		if err != nil {
			return xerrors.Errorf("couldn't write accepted: %v", err)
		}
	}

	return nil
}

// Serialize implements serde.Message. It returns the serialized data.
func (d Data) Serialize(ctx serde.Context) ([]byte, error) {
	format := dataFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, d)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type ResultKey struct{}

type DataFactory struct {
	fac serde.Factory
}

func NewDataFactory(f tap.TransactionFactory) DataFactory {
	return DataFactory{
		fac: NewResultFactory(f),
	}
}

func (f DataFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return f.DataOf(ctx, data)
}

func (f DataFactory) DataOf(ctx serde.Context, data []byte) (validation.Data, error) {
	format := dataFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, ResultKey{}, f.fac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	vdata, ok := msg.(Data)
	if !ok {
		return nil, xerrors.Errorf("invalid data type")
	}

	return vdata, nil
}
