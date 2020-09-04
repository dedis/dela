package anon

import (
	"encoding/binary"
	"io"

	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var txFormats = registry.NewSimpleRegistry()

// RegisterTransactionFormat registers the engine for the provided format.
func RegisterTransactionFormat(f serde.Format, e serde.FormatEngine) {
	txFormats.Register(f, e)
}

// Transaction is a an anonymous transaction. It can contain arguments but the
// identity will always be nil.
//
// - implements txn.Transaction
type Transaction struct {
	nonce uint64
	args  map[string][]byte
	hash  []byte
}

type template struct {
	Transaction

	hashFactory crypto.HashFactory
}

// TransactionOption is the type of options to create a transaction.
type TransactionOption func(*template)

// WithArg is an option to set an argument with the key and the value.
func WithArg(key string, value []byte) TransactionOption {
	return func(tmpl *template) {
		tmpl.args[key] = value
	}
}

// WithHashFactory is an option to set a different hash factory when creating a
// transaction.
func WithHashFactory(f crypto.HashFactory) TransactionOption {
	return func(tmpl *template) {
		tmpl.hashFactory = f
	}
}

// NewTransaction creates a new transaction with the provided nonce.
func NewTransaction(nonce uint64, opts ...TransactionOption) (Transaction, error) {
	tmpl := template{
		Transaction: Transaction{
			nonce: nonce,
			args:  make(map[string][]byte),
		},
		hashFactory: crypto.NewSha256Factory(),
	}

	for _, opt := range opts {
		opt(&tmpl)
	}

	h := tmpl.hashFactory.New()
	err := tmpl.Fingerprint(h)
	if err != nil {
		return tmpl.Transaction, xerrors.Errorf("couldn't fingerprint tx: %v", err)
	}

	tmpl.hash = h.Sum(nil)

	return tmpl.Transaction, nil
}

// GetID implements txn.Transaction. It returns the ID of the transaction.
func (t Transaction) GetID() []byte {
	return t.hash
}

// GetNonce returns the nonce of the transaction.
func (t Transaction) GetNonce() uint64 {
	return t.nonce
}

// GetIdentity implements txn.Transaction. It returns nil.
func (t Transaction) GetIdentity() access.Identity {
	return nil
}

// GetArgs returns the list of arguments available.
func (t Transaction) GetArgs() []string {
	args := make([]string, 0, len(t.args))
	for key := range t.args {
		args = append(args, key)
	}

	return args
}

// GetArg implements txn.Transaction. It returns the value of the argument if it
// is set, otherwise nil.
func (t Transaction) GetArg(key string) []byte {
	return t.args[key]
}

// Fingerprint implements serde.Fingerprinter. It writes a deterministic binary
// representation of the transaction.
func (t Transaction) Fingerprint(w io.Writer) error {
	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, t.nonce)

	_, err := w.Write(buffer)
	if err != nil {
		return xerrors.Errorf("couldn't write nonce: %v", err)
	}

	return nil
}

// Serialize implements serde.Message. It returns the serialized data of the
// transaction.
func (t Transaction) Serialize(ctx serde.Context) ([]byte, error) {
	format := txFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, t)
	if err != nil {
		return nil, xerrors.Errorf("failed to encode: %v", err)
	}

	return data, nil
}

// TransactionFactory is a factory to deserialize transactions.
//
// - implements serde.Factory
type TransactionFactory struct{}

// NewTransactionFactory returns a new factory.
func NewTransactionFactory() TransactionFactory {
	return TransactionFactory{}
}

// Deserialize implements serde.Factory. It populates the transaction from the
// data if appropriate, otherwise it returns an error.
func (f TransactionFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return f.TransactionOf(ctx, data)
}

// TransactionOf implements txn.TransactionFactory. It populates the transaction
// from the data if appropriate, otherwise it returns an error.
func (f TransactionFactory) TransactionOf(ctx serde.Context, data []byte) (txn.Transaction, error) {
	format := txFormats.Get(ctx.GetFormat())

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("failed to decode: %v", err)
	}

	tx, ok := msg.(Transaction)
	if !ok {
		return nil, xerrors.Errorf("invalid transaction of type '%T'", msg)
	}

	return tx, nil
}

// TransactionManager is a manager to create anonymous transactions. It manages
// the nonce by itself.
//
// - implements txn.TransactionManager
type transactionManager struct {
	nonce uint64
}

// NewManager creates a new transaction manager.
func NewManager() txn.TransactionManager {
	return &transactionManager{
		// TODO: sync with latest block
		nonce: 0,
	}
}

// Make creates a transaction populated with the arguments.
func (mgr *transactionManager) Make(args ...txn.Arg) (txn.Transaction, error) {
	opts := make([]TransactionOption, len(args))
	for i, arg := range args {
		opts[i] = WithArg(arg.Key, arg.Value)
	}

	tx, err := NewTransaction(mgr.nonce, opts...)
	if err != nil {
		return nil, err
	}

	return tx, nil
}
