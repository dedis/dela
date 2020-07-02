// Package basic implements a kind of transaction that includes a signature and
// a nonce so that it can prevent replay attacks. Access control can also be
// enforced from the identity of the transaction.
//
// The task defines how the transaction will be consumed and it follows the same
// separation logic with a client and a server side. The client only creates the
// task with its arguments and the server will decorate it to consume it.
package basic

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"reflect"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/common"
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/ledger/transactions"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var txFormats = registry.NewSimpleRegistry()

// RegisterTxFormat registers the engine for the provided format.
func RegisterTxFormat(c serde.Format, f serde.FormatEngine) {
	txFormats.Register(c, f)
}

// ClientTask is a task inside a transaction.
type ClientTask interface {
	serde.Message
	serde.Fingerprinter
}

// Context is the context provided to a server transaction when consumed.
type Context interface {
	// GetID returns the unique identifier of the transaction.
	GetID() []byte

	// GetIdentity returns the identity who signed the transaction.
	GetIdentity() arc.Identity
}

// ServerTask is an extension of the client task that can be consumed to update
// the state of an inventory.
type ServerTask interface {
	ClientTask

	Consume(Context, inventory.WritablePage) error
}

// ClientTransaction is an implementation of the client ClientTransaction that
// is using a signature to determine the identity belonging to it. It also wraps
// a task that will be executed.
//
// - implements transactions.ClientTransaction
type ClientTransaction struct {
	hash      []byte
	nonce     uint64
	identity  crypto.PublicKey
	signature crypto.Signature
	task      ClientTask
}

// GetID implements transactions.ClientTransaction. It returns the unique
// identifier of the transaction.
func (t ClientTransaction) GetID() []byte {
	return t.hash[:]
}

// GetNonce returns the nonce of the transaction.
func (t ClientTransaction) GetNonce() uint64 {
	return t.nonce
}

// GetIdentity implements basic.Context. It returns the identity who signed the
// transaction.
func (t ClientTransaction) GetIdentity() arc.Identity {
	return t.identity
}

// GetSignature returns the signature transaction that will prove the
// authenticity of the identity.
func (t ClientTransaction) GetSignature() crypto.Signature {
	return t.signature
}

// GetTask returns the task of the transaction.
func (t ClientTransaction) GetTask() ClientTask {
	return t.task
}

// Serialize implements serde.Message.
func (t ClientTransaction) Serialize(ctx serde.Context) ([]byte, error) {
	format := txFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, t)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode tx: %v", err)
	}

	return data, nil
}

// Fingerprint implements encoding.Fingerprinter. It serializes the transaction
// into the writer in a deterministic way.
func (t ClientTransaction) Fingerprint(w io.Writer) error {
	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer[:], t.nonce)

	_, err := w.Write(buffer)
	if err != nil {
		return xerrors.Errorf("couldn't write nonce: %v", err)
	}

	buffer, err = t.identity.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("couldn't marshal identity: %v", err)
	}

	_, err = w.Write(buffer)
	if err != nil {
		return xerrors.Errorf("couldn't write identity: %v", err)
	}

	err = t.task.Fingerprint(w)
	if err != nil {
		return xerrors.Errorf("couldn't write task: %v", err)
	}

	return nil
}

// String implements fmt.Stringer. It returns a string representation of the
// transaction.
func (t ClientTransaction) String() string {
	return fmt.Sprintf("Transaction[%x]@%v", t.hash, t.identity)
}

// ServerTransaction is an extension of the transaction that can be consumed.
//
// - implements transactions.ServerTransaction
type ServerTransaction struct {
	ClientTransaction
}

type serverTransactionTemplate struct {
	ClientTransaction
	hashFactory crypto.HashFactory
}

// ServerTransactionOption is the type of an option when creating a new server
// transaction.
type ServerTransactionOption func(*serverTransactionTemplate)

// WithNonce is an option to set the nonce of the transaction.
func WithNonce(nonce uint64) ServerTransactionOption {
	return func(tmpl *serverTransactionTemplate) {
		tmpl.nonce = nonce
	}
}

// WithIdentity is an option to set the identity of the transaction.
func WithIdentity(ident crypto.PublicKey, sig crypto.Signature) ServerTransactionOption {
	return func(tmpl *serverTransactionTemplate) {
		tmpl.identity = ident
		tmpl.signature = sig
	}
}

// WithTask is an option to set the task of a transaction.
func WithTask(task ServerTask) ServerTransactionOption {
	return func(tmpl *serverTransactionTemplate) {
		tmpl.task = task
	}
}

// WithHashFactory is an option to set the hash factory to generate the
// fingerprint of the transaction.
func WithHashFactory(f crypto.HashFactory) ServerTransactionOption {
	return func(tmpl *serverTransactionTemplate) {
		tmpl.hashFactory = f
	}
}

// WithNoFingerprint is an option to disable the calculation of the fingerprint
// when instantiating the transaction. This is mostly useful for testing.
func WithNoFingerprint() ServerTransactionOption {
	return func(tmpl *serverTransactionTemplate) {
		tmpl.hashFactory = nil
	}
}

// NewServerTransaction creates a new server transaction.
func NewServerTransaction(opts ...ServerTransactionOption) (ServerTransaction, error) {
	tmpl := serverTransactionTemplate{
		hashFactory: crypto.NewSha256Factory(),
	}

	for _, opt := range opts {
		opt(&tmpl)
	}

	tx := ServerTransaction{
		ClientTransaction: tmpl.ClientTransaction,
	}

	if tmpl.hashFactory == nil {
		// No hash factory means the fingerprint should be skipped. In that case
		// there is no verification of the integrity of the tx.
		return tx, nil
	}

	// Before going to fingerprint, some fields are necessary.
	if tmpl.task == nil {
		return tx, xerrors.New("task is nil")
	}

	if tmpl.identity == nil {
		return tx, xerrors.New("identity is nil")
	}

	h := tmpl.hashFactory.New()
	err := tmpl.Fingerprint(h)
	if err != nil {
		return tx, xerrors.Errorf("couldn't fingerprint tx: %v", err)
	}

	tx.hash = h.Sum(nil)

	return tx, nil
}

// Consume implements transactions.ServerTransaction. It first insures the nonce
// is correct and writes the new one into the page. It then consumes the task of
// the transaction.
func (t ServerTransaction) Consume(page inventory.WritablePage) error {
	// TODO: consume nonce

	task, ok := t.task.(ServerTask)
	if !ok {
		return xerrors.Errorf("task must implement 'basic.ServerTask'")
	}

	err := task.Consume(t, page)
	if err != nil {
		return xerrors.Errorf("couldn't consume task '%T': %v", task, err)
	}

	return nil
}

// IdentityKey is the key of an identity factory.
type IdentityKey struct{}

// SignatureKey is the key of a signature factory.
type SignatureKey struct{}

// TaskKey is the key of a task factory.
type TaskKey struct{}

// KeyOf returns the unique string for a message.
func KeyOf(m serde.Message) string {
	typ := reflect.TypeOf(m)
	return fmt.Sprintf("%s.%s", typ.PkgPath(), typ.Name())
}

// TransactionFactory is an implementation of a Byzcoin transaction factory.
//
// - implements ledger.TxFactory
// - implements serde.Factory
type TransactionFactory struct {
	signer           crypto.Signer
	hashFactory      crypto.HashFactory
	publicKeyFactory crypto.PublicKeyFactory
	signatureFactory crypto.SignatureFactory
	registry         map[string]serde.Factory
}

// NewTransactionFactory returns a new instance of the transaction factory.
func NewTransactionFactory(signer crypto.Signer) TransactionFactory {
	return TransactionFactory{
		signer:           signer,
		hashFactory:      crypto.NewSha256Factory(),
		publicKeyFactory: common.NewPublicKeyFactory(),
		signatureFactory: common.NewSignatureFactory(),
		registry:         make(map[string]serde.Factory),
	}
}

// Register registers the message to use the given factory to deserialize.
func (f TransactionFactory) Register(m serde.Message, factory serde.Factory) {
	f.registry[KeyOf(m)] = factory
}

// Get returns the factory of the provided key or nil.
func (f TransactionFactory) Get(key string) serde.Factory {
	return f.registry[key]
}

// New returns a new transaction from the given task. The transaction will be
// signed.
func (f TransactionFactory) New(task ClientTask) (transactions.ClientTransaction, error) {
	tx := ClientTransaction{
		nonce:    rand.Uint64(), // TODO: monotonic nonce
		identity: f.signer.GetPublicKey(),
		task:     task,
	}

	h := f.hashFactory.New()
	err := tx.Fingerprint(h)
	if err != nil {
		return tx, xerrors.Errorf("couldn't compute hash: %v", err)
	}

	tx.hash = h.Sum(nil)

	tx.signature, err = f.signer.Sign(tx.hash)
	if err != nil {
		return tx, xerrors.Errorf("couldn't sign tx: %v", err)
	}

	return tx, nil
}

// Deserialize implements serde.Factory. It looks up the format for the provided
// context and returns the message associated with the data if appropriate,
// otherwise it returns an error.
func (f TransactionFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := txFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, IdentityKey{}, f.publicKeyFactory)
	ctx = serde.WithFactory(ctx, SignatureKey{}, f.signatureFactory)
	ctx = serde.WithFactory(ctx, TaskKey{}, f)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode tx: %v", err)
	}

	tx, ok := msg.(ServerTransaction)
	if !ok {
		return nil, xerrors.Errorf("invalid tx type '%T'", msg)
	}

	err = tx.identity.Verify(tx.hash, tx.signature)
	if err != nil {
		return nil, xerrors.Errorf("signature does not match tx: %v", err)
	}

	return tx, nil
}

// TxOf implements transactions.TxFactory. It deserializes the message and cast
// it to a transaction type.
func (f TransactionFactory) TxOf(ctx serde.Context, data []byte) (transactions.ServerTransaction, error) {
	msg, err := f.Deserialize(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg.(ServerTransaction), nil
}
