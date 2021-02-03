// Package signed is an implementation of the transaction abstraction.
//
// It uses a signature to make sure the identity owns the transaction. The nonce
// is a monotonically increasing number that is used to prevent a replay attack
// of an existing transaction.
//
// Documentation Last Review: 08.10.2020
//
package signed

import (
	"encoding/binary"
	"io"
	"sort"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/common"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var txFormats = registry.NewSimpleRegistry()

// RegisterTransactionFormat registers the engine for the provided format.
func RegisterTransactionFormat(f serde.Format, e serde.FormatEngine) {
	txFormats.Register(f, e)
}

// Transaction is a signed transaction using a nonce to protect itself against
// replay attack.
//
// - implements txn.Transaction
type Transaction struct {
	nonce  uint64
	args   map[string][]byte
	pubkey crypto.PublicKey
	sig    crypto.Signature
	hash   []byte
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

// WithSignature is an option to set a valid signature. The signature will be
// verified against the identity.
func WithSignature(sig crypto.Signature) TransactionOption {
	return func(tmpl *template) {
		tmpl.sig = sig
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
func NewTransaction(nonce uint64, pk crypto.PublicKey, opts ...TransactionOption) (*Transaction, error) {
	tmpl := template{
		Transaction: Transaction{
			nonce:  nonce,
			pubkey: pk,
			args:   make(map[string][]byte),
		},
		hashFactory: crypto.NewSha256Factory(),
	}

	for _, opt := range opts {
		opt(&tmpl)
	}

	h := tmpl.hashFactory.New()
	err := tmpl.Fingerprint(h)
	if err != nil {
		return nil, xerrors.Errorf("couldn't fingerprint tx: %v", err)
	}

	tmpl.hash = h.Sum(nil)

	if tmpl.sig != nil {
		err := tmpl.pubkey.Verify(tmpl.hash, tmpl.sig)
		if err != nil {
			return nil, xerrors.Errorf("invalid signature: %v", err)
		}
	}

	return &tmpl.Transaction, nil
}

// GetID implements txn.Transaction. It returns the ID of the transaction.
func (t *Transaction) GetID() []byte {
	return t.hash
}

// GetNonce implements txn.Transaction. It returns the nonce of the transaction.
func (t *Transaction) GetNonce() uint64 {
	return t.nonce
}

// GetIdentity implements txn.Transaction. It returns nil.
func (t *Transaction) GetIdentity() access.Identity {
	return t.pubkey
}

// GetSignature returns the signature of the transaction.
func (t *Transaction) GetSignature() crypto.Signature {
	return t.sig
}

// GetArgs returns the list of arguments available.
func (t *Transaction) GetArgs() []string {
	args := make([]string, 0, len(t.args))
	for key := range t.args {
		args = append(args, key)
	}

	return args
}

// GetArg implements txn.Transaction. It returns the value of the argument if it
// is set, otherwise nil.
func (t *Transaction) GetArg(key string) []byte {
	return t.args[key]
}

// Sign signs the transaction and stores the signature.
func (t *Transaction) Sign(signer crypto.Signer) error {
	if len(t.hash) == 0 {
		return xerrors.New("missing digest in transaction")
	}

	if !signer.GetPublicKey().Equal(t.pubkey) {
		return xerrors.New("mismatch signer and identity")
	}

	sig, err := signer.Sign(t.hash)
	if err != nil {
		return xerrors.Errorf("signer: %v", err)
	}

	t.sig = sig

	return nil
}

// Fingerprint implements serde.Fingerprinter. It writes a deterministic binary
// representation of the transaction.
func (t *Transaction) Fingerprint(w io.Writer) error {
	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, t.nonce)

	_, err := w.Write(buffer)
	if err != nil {
		return xerrors.Errorf("couldn't write nonce: %v", err)
	}

	// Sort the argument to deterministically write them to the hash.
	args := make(sort.StringSlice, 0, len(t.args))
	for key := range t.args {
		args = append(args, key)
	}

	sort.Sort(args)

	for _, key := range args {
		_, err = w.Write(append([]byte(key), t.args[key]...))
		if err != nil {
			return xerrors.Errorf("couldn't write arg: %v", err)
		}
	}

	buffer, err = t.pubkey.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to marshal public key: %v", err)
	}

	_, err = w.Write(buffer)
	if err != nil {
		return xerrors.Errorf("couldn't write public key: %v", err)
	}

	return nil
}

// Serialize implements serde.Message. It returns the serialized data of the
// transaction.
func (t *Transaction) Serialize(ctx serde.Context) ([]byte, error) {
	format := txFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, t)
	if err != nil {
		return nil, xerrors.Errorf("failed to encode: %v", err)
	}

	return data, nil
}

// PublicKeyFac is the key of the public key factory.
type PublicKeyFac struct{}

// SignatureFac is the key of the signature factory.
type SignatureFac struct{}

// TransactionFactory is a factory to deserialize transactions.
//
// - implements serde.Factory
type TransactionFactory struct {
	pubkeyFac common.PublicKeyFactory
	sigFac    common.SignatureFactory
}

// NewTransactionFactory returns a new factory.
func NewTransactionFactory() TransactionFactory {
	return TransactionFactory{
		pubkeyFac: common.NewPublicKeyFactory(),
		sigFac:    common.NewSignatureFactory(),
	}
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

	ctx = serde.WithFactory(ctx, PublicKeyFac{}, f.pubkeyFac)
	ctx = serde.WithFactory(ctx, SignatureFac{}, f.sigFac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("failed to decode: %v", err)
	}

	tx, ok := msg.(*Transaction)
	if !ok {
		return nil, xerrors.Errorf("invalid transaction of type '%T'", msg)
	}

	return tx, nil
}

// Client is the interface the manager is using to get the nonce of an identity.
// It allows a local implementation, or through a network client.
type Client interface {
	GetNonce(access.Identity) (uint64, error)
}

// TransactionManager is a manager to create signed transactions. It manages the
// nonce by itself, except if the transaction is refused by the ledger. In that
// case the manager should be synchronized before creating a new one.
//
// - implements txn.Manager
type TransactionManager struct {
	client  Client
	signer  crypto.Signer
	nonce   uint64
	hashFac crypto.HashFactory
}

// NewManager creates a new transaction manager.
//
// - implements txn.Manager
func NewManager(signer crypto.Signer, client Client) *TransactionManager {
	return &TransactionManager{
		client:  client,
		signer:  signer,
		nonce:   0,
		hashFac: crypto.NewSha256Factory(),
	}
}

// Make implements txn.Manager. It creates a transaction populated with the
// arguments.
func (mgr *TransactionManager) Make(args ...txn.Arg) (txn.Transaction, error) {
	opts := make([]TransactionOption, len(args), len(args)+1)
	for i, arg := range args {
		opts[i] = WithArg(arg.Key, arg.Value)
	}

	opts = append(opts, WithHashFactory(mgr.hashFac))

	tx, err := NewTransaction(mgr.nonce, mgr.signer.GetPublicKey(), opts...)
	if err != nil {
		return nil, xerrors.Errorf("failed to create tx: %v", err)
	}

	err = tx.Sign(mgr.signer)
	if err != nil {
		return nil, xerrors.Errorf("failed to sign: %v", err)
	}

	mgr.nonce++

	return tx, nil
}

// Sync implements txn.Manager. It fetches the latest nonce of the signer to
// create valid transactions.
func (mgr *TransactionManager) Sync() error {
	nonce, err := mgr.client.GetNonce(mgr.signer.GetPublicKey())
	if err != nil {
		return xerrors.Errorf("client: %v", err)
	}

	mgr.nonce = nonce

	dela.Logger.Debug().Uint64("nonce", nonce).Msg("manager synchronized")

	return nil
}
