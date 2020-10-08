// This file contains the implementation of the result returned by the service.
//
// Documentation Last Review: 08.10.2020
//

package simple

import (
	"io"

	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var (
	txResFormats = registry.NewSimpleRegistry()
	resFormats   = registry.NewSimpleRegistry()
)

// RegisterTransactionResultFormat registers the engine for the provided format.
func RegisterTransactionResultFormat(f serde.Format, e serde.FormatEngine) {
	txResFormats.Register(f, e)
}

// RegisterResultFormat registers the engine for the provided format.
func RegisterResultFormat(f serde.Format, e serde.FormatEngine) {
	resFormats.Register(f, e)
}

// TransactionResult is the result of a transaction processing. It contains the
// transaction and its state of success.
//
// - implements validation.TransactionResult
type TransactionResult struct {
	tx       txn.Transaction
	accepted bool
	reason   string
}

// NewTransactionResult creates a new transaction result for the provided
// transaction.
func NewTransactionResult(tx txn.Transaction, accepted bool, reason string) TransactionResult {
	return TransactionResult{
		tx:       tx,
		accepted: accepted,
		reason:   reason,
	}
}

// GetTransaction implements validation.TransactionResult. It returns the
// transaction associated to the result.
func (res TransactionResult) GetTransaction() txn.Transaction {
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
	format := txResFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, res)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

// TransactionKey is the key of the transaction factory.
type TransactionKey struct{}

// TransactionResultFactory is the factory to deserialize transaction results.
//
// - implements serde.Factory
type TransactionResultFactory struct {
	fac txn.Factory
}

// NewTransactionResultFactory creates a new transaction result factory.
func NewTransactionResultFactory(f txn.Factory) TransactionResultFactory {
	return TransactionResultFactory{
		fac: f,
	}
}

// Deserialize implements serde.Factory. It populates the transaction result if
// appropriate, otherwise it returns an error.
func (f TransactionResultFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := txResFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, TransactionKey{}, f.fac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("decoding failed: %v", err)
	}

	return msg, nil
}

// Result is the result of a standard validation.
//
// - implements validation.Result
type Result struct {
	txs []TransactionResult
}

// NewResult creates a new result from a list of transaction results.
func NewResult(results []TransactionResult) Result {
	return Result{
		txs: results,
	}
}

// GetTransactionResults implements validation.Result. It returns the
// transaction results.
func (d Result) GetTransactionResults() []validation.TransactionResult {
	res := make([]validation.TransactionResult, len(d.txs))
	for i, r := range d.txs {
		res[i] = r
	}

	return res
}

// Fingerprint implements serde.Fingerprinter. It writes a deterministic binary
// representation of the result.
func (d Result) Fingerprint(w io.Writer) error {
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

// Serialize implements serde.Message. It returns the serialized data of the
// result.
func (d Result) Serialize(ctx serde.Context) ([]byte, error) {
	format := resFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, d)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

// ResultKey is the key of the transaction result factory.
type ResultKey struct{}

// ResultFactory is the factory to deserialize results.
//
// - implements validation.ResultFactory
type ResultFactory struct {
	fac serde.Factory
}

// NewResultFactory creates a new result factory.
func NewResultFactory(f txn.Factory) ResultFactory {
	return ResultFactory{
		fac: NewTransactionResultFactory(f),
	}
}

// Deserialize implements serde.Factory. It populates the result if appropriate,
// otherwise it returns an error.
func (f ResultFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return f.ResultOf(ctx, data)
}

// ResultOf implements validation.ResultFactory. It returns the result from the
// serialized data if appropriate, otherwise it returns an error.
func (f ResultFactory) ResultOf(ctx serde.Context, data []byte) (validation.Result, error) {
	format := resFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, ResultKey{}, f.fac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("decoding failed: %v", err)
	}

	res, ok := msg.(Result)
	if !ok {
		return nil, xerrors.Errorf("invalid result type '%T'", msg)
	}

	return res, nil
}
