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

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/common"
	"go.dedis.ch/dela/encoding"
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
	encoding.Packable
	encoding.Fingerprinter
	serde.Message
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

// TaskFactory provide the primitives to instantiate a task from its protobuf
// message.
type TaskFactory interface {
	serde.Factory

	FromProto(proto.Message) (ServerTask, error)
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

// Pack implements encoding.Packable. It returns the protobuf message of the
// transaction.
func (t transaction) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &TransactionProto{
		Nonce: t.nonce,
	}

	var err error
	pb.Identity, err = enc.PackAny(t.identity)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack identity: %v", err)
	}

	pb.Signature, err = enc.PackAny(t.signature)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack signature: %v", err)
	}

	pb.Task, err = enc.PackAny(t.task)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack task: %v", err)
	}

	pb.Type = keyOf(t.task)

	return pb, nil
}

// VisitJSON implements serde.Message. It returns the JSON message for this
// transaction.
func (t transaction) VisitJSON(ser serde.Serializer) (interface{}, error) {
	identity, err := ser.Serialize(t.identity)
	if err != nil {
		return nil, err
	}

	signature, err := ser.Serialize(t.signature)
	if err != nil {
		return nil, err
	}

	task, err := ser.Serialize(t.task)
	if err != nil {
		return nil, err
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
func (t transaction) Fingerprint(w io.Writer, enc encoding.ProtoMarshaler) error {
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

	err = t.task.Fingerprint(w, enc)
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
	publicKeyFactory crypto.PublicKeyFactory
	signatureFactory crypto.SignatureFactory
	registry         map[string]TaskFactory
	encoder          encoding.ProtoMarshaler
}

// NewTransactionFactory returns a new instance of the transaction factory.
func NewTransactionFactory(signer crypto.Signer) TransactionFactory {
	return TransactionFactory{
		signer:           signer,
		hashFactory:      crypto.NewSha256Factory(),
		publicKeyFactory: common.NewPublicKeyFactory(),
		signatureFactory: common.NewSignatureFactory(),
		registry:         make(map[string]TaskFactory),
		encoder:          encoding.NewProtoEncoder(),
	}
}

// Register registers the message to use the given factory to deserialize.
func (f TransactionFactory) Register(m serde.Message, factory TaskFactory) {
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
	err := tx.Fingerprint(h, f.encoder)
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

// FromProto implements ledger.TransactionFactory. It returns a new transaction
// built from the protobuf message.
func (f TransactionFactory) FromProto(in proto.Message) (transactions.ServerTransaction, error) {
	var pb *TransactionProto

	switch msg := in.(type) {
	case *any.Any:
		pb = &TransactionProto{}
		err := f.encoder.UnmarshalAny(msg, pb)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal input: %v", err)
		}
	case *TransactionProto:
		pb = msg
	default:
		return nil, xerrors.Errorf("invalid transaction type '%T'", in)
	}

	factory := f.registry[pb.Type]
	if factory == nil {
		return nil, xerrors.Errorf("missing factory for type '%s'", pb.Type)
	}

	msg, err := f.encoder.UnmarshalDynamicAny(pb.GetTask())
	if err != nil {
		return nil, err
	}

	task, err := factory.FromProto(msg)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode task: %v", err)
	}

	tx := serverTransaction{
		transaction: transaction{
			nonce: pb.GetNonce(),
			task:  task,
		},
	}

	err = f.fillIdentity(&tx, pb)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// VisitJSON implements serde.Factory. It returns the transaction associated
// with the input if appropriate, otherwise it returns an error.
func (f TransactionFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Transaction{}
	err := in.Feed(&m)
	if err != nil {
		return nil, err
	}

	var identity crypto.PublicKey
	err = in.GetSerializer().Deserialize(m.Identity, f.publicKeyFactory, &identity)
	if err != nil {
		return nil, err
	}

	var signature crypto.Signature
	err = in.GetSerializer().Deserialize(m.Signature, f.signatureFactory, &signature)
	if err != nil {
		return nil, err
	}

	factory := f.registry[m.Task.Type]
	if factory == nil {
		return nil, xerrors.Errorf("missing factory for type '%s'", m.Task.Type)
	}

	var task ServerTask
	err = in.GetSerializer().Deserialize(m.Task.Value, factory, &task)
	if err != nil {
		return nil, err
	}

	tx := transaction{
		nonce:     m.Nonce,
		identity:  identity,
		signature: signature,
		task:      task,
	}

	h := f.hashFactory.New()
	err = tx.Fingerprint(h, f.encoder)
	if err != nil {
		return nil, err
	}

	tx.hash = h.Sum(nil)

	err = identity.Verify(tx.hash, signature)
	if err != nil {
		return nil, xerrors.Errorf("signature does not match tx: %v", err)
	}

	return tx, nil
}

func (f TransactionFactory) fillIdentity(tx *serverTransaction, pb *TransactionProto) error {
	identity, err := f.publicKeyFactory.FromProto(pb.GetIdentity())
	if err != nil {
		return xerrors.Errorf("couldn't decode public key: %v", err)
	}

	signature, err := f.signatureFactory.FromProto(pb.GetSignature())
	if err != nil {
		return xerrors.Errorf("couldn't decode signature: %v", err)
	}

	tx.identity = identity
	tx.signature = signature

	h := f.hashFactory.New()
	err = tx.Fingerprint(h, f.encoder)
	if err != nil {
		return xerrors.Errorf("couldn't compute hash: %v", err)
	}

	tx.hash = h.Sum(nil)

	err = tx.identity.Verify(tx.hash, tx.signature)
	if err != nil {
		return xerrors.Errorf("signature does not match tx: %v", err)
	}

	return nil
}

func keyOf(m serde.Message) string {
	typ := reflect.TypeOf(m)
	return fmt.Sprintf("%s.%s", typ.PkgPath(), typ.Name())
}
