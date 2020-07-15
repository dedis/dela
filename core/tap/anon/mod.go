package anon

import (
	"encoding/binary"
	"io"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/ledger/arc"
	"golang.org/x/xerrors"
)

// Transaction is a an anonymous transaction. It can contain arguments but the
// identity will always be nil.
//
// - implements tap.Transaction
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

// GetID implements tap.Transaction. It returns the ID of the transaction.
func (t Transaction) GetID() []byte {
	return t.hash
}

// GetIdentity implements tap.Transaction. It returns nil.
func (t Transaction) GetIdentity() arc.Identity {
	return nil
}

// GetArg implements tap.Transaction. It returns the value of the argument if it
// is set, otherwise nil.
func (t Transaction) GetArg(key string) []byte {
	return t.args[key]
}

// Fingerprint implements serde.Fingerprinter. It writes a deteministic binary
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
