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
	"go.dedis.ch/dela/ledger/transactions/basic/json"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

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

// transaction is an implementation of the client transaction that is using a
// signature to determine the identity belonging to it. It also wraps a task
// that will be executed.
//
// - implements transactions.ClientTransaction
type transaction struct {
	serde.UnimplementedMessage

	hash      []byte
	nonce     uint64
	identity  crypto.PublicKey
	signature crypto.Signature
	task      ClientTask
}

// GetID implements transactions.ClientTransaction. It returns the unique
// identifier of the transaction.
func (t transaction) GetID() []byte {
	return t.hash[:]
}

// GetIdentity implements basic.Context. It returns the identity who signed the
// transaction.
func (t transaction) GetIdentity() arc.Identity {
	return t.identity
}

// VisitJSON implements serde.Message. It returns the JSON message for this
// transaction.
func (t transaction) VisitJSON(ser serde.Serializer) (interface{}, error) {
	identity, err := ser.Serialize(t.identity)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize identity: %v", err)
	}

	signature, err := ser.Serialize(t.signature)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize signature: %v", err)
	}

	task, err := ser.Serialize(t.task)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize task: %v", err)
	}

	m := json.Transaction{
		Nonce:     t.nonce,
		Identity:  identity,
		Signature: signature,
		Task: json.Task{
			Type:  keyOf(t.task),
			Value: task,
		},
	}

	return m, nil
}

// Fingerprint implements encoding.Fingerprinter. It serializes the transaction
// into the writer in a deterministic way.
func (t transaction) Fingerprint(w io.Writer) error {
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
func (t transaction) String() string {
	return fmt.Sprintf("Transaction[%x]@%v", t.hash, t.identity)
}

// serverTransaction is an extension of the transaction that can be consumed.
//
// - implements transactions.ServerTransaction
type serverTransaction struct {
	transaction
}

// Consume implements transactions.ServerTransaction. It first insures the nonce
// is correct and writes the new one into the page. It then consumes the task of
// the transaction.
func (t serverTransaction) Consume(page inventory.WritablePage) error {
	// TODO: consume nonce

	task, ok := t.task.(ServerTask)
	if !ok {
		return xerrors.Errorf("task must implement 'basic.ServerTask'")
	}

	err := task.Consume(t, page)
	if err != nil {
		return xerrors.Errorf("couldn't consume task: %v", err)
	}

	return nil
}

// TransactionFactory is an implementation of a Byzcoin transaction factory.
//
// - implements ledger.TransactionFactory
type TransactionFactory struct {
	serde.UnimplementedFactory

	signer           crypto.Signer
	hashFactory      crypto.HashFactory
	publicKeyFactory serde.Factory
	signatureFactory serde.Factory
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
	f.registry[keyOf(m)] = factory
}

// New returns a new transaction from the given task. The transaction will be
// signed.
func (f TransactionFactory) New(task ClientTask) (transactions.ClientTransaction, error) {
	tx := transaction{
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

// VisitJSON implements serde.Factory. It returns the transaction associated
// with the input if appropriate, otherwise it returns an error.
func (f TransactionFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Transaction{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize transaction: %v", err)
	}

	var identity crypto.PublicKey
	err = in.GetSerializer().Deserialize(m.Identity, f.publicKeyFactory, &identity)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize identity: %v", err)
	}

	var signature crypto.Signature
	err = in.GetSerializer().Deserialize(m.Signature, f.signatureFactory, &signature)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize signature: %v", err)
	}

	factory := f.registry[m.Task.Type]
	if factory == nil {
		return nil, xerrors.Errorf("unknown factory for type '%s'", m.Task.Type)
	}

	var task ServerTask
	err = in.GetSerializer().Deserialize(m.Task.Value, factory, &task)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize task: %v", err)
	}

	tx := serverTransaction{
		transaction: transaction{
			nonce:     m.Nonce,
			identity:  identity,
			signature: signature,
			task:      task,
		},
	}

	h := f.hashFactory.New()
	err = tx.Fingerprint(h)
	if err != nil {
		return nil, xerrors.Errorf("couldn't fingerprint: %v", err)
	}

	tx.hash = h.Sum(nil)

	err = identity.Verify(tx.hash, signature)
	if err != nil {
		return nil, xerrors.Errorf("signature does not match tx: %v", err)
	}

	return tx, nil
}

func keyOf(m serde.Message) string {
	typ := reflect.TypeOf(m)
	return fmt.Sprintf("%s.%s", typ.PkgPath(), typ.Name())
}
