package smartcontract

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/ledger"
	"golang.org/x/xerrors"
)

// Transaction is an atomic execution.
//
// - implements ledger.Transaction
// - implements encoding.Packable
// - implements fmt.Stringer
type Transaction struct {
	hash []byte
	// TODO: smart contract input
	value string
}

func newTransaction(factory crypto.HashFactory, value string) (Transaction, error) {
	tx := Transaction{value: value}

	var err error
	tx.hash, err = tx.computeHash(factory)
	if err != nil {
		return tx, xerrors.Errorf("couldn't hash the tx: %v", err)
	}

	return tx, nil
}

// GetID implements ledger.Transaction. It returns the unique identifier of the
// transaction.
func (t Transaction) GetID() []byte {
	return t.hash[:]
}

// Pack implements encoding.Packable. It returns the protobuf message of a
// transaction.
func (t Transaction) Pack() (proto.Message, error) {
	return &TransactionProto{Value: t.value}, nil
}

func (t Transaction) MarshalText() ([]byte, error) {
	return []byte(t.value), nil
}

func (t Transaction) String() string {
	return fmt.Sprintf("Transaction{Value: %s}", t.value)
}

func (t Transaction) computeHash(f crypto.HashFactory) ([]byte, error) {
	h := f.New()
	_, err := h.Write([]byte(t.value))
	if err != nil {
		return nil, xerrors.Errorf("couldn't write t.value: %v", err)
	}

	return h.Sum(nil), nil
}

// TransactionFactory is an implementation of a Byzcoin transaction factory.
//
// - implements ledger.TransactionFactory
type TransactionFactory struct {
	hashFactory crypto.HashFactory
}

func NewTransactionFactory() TransactionFactory {
	return TransactionFactory{
		hashFactory: crypto.NewSha256Factory(),
	}
}

func (f TransactionFactory) New(value string) (Transaction, error) {
	return newTransaction(f.hashFactory, value)
}

// FromProto implements ledger.TransactionFactory. It returns a new transaction
// built from the protobuf message.
func (f TransactionFactory) FromProto(pb proto.Message) (ledger.Transaction, error) {
	var msg *TransactionProto
	switch in := pb.(type) {
	case *TransactionProto:
		msg = in
	default:
		return nil, xerrors.Errorf("invalid message type '%T'", pb)
	}

	tx := Transaction{
		value: msg.GetValue(),
	}

	var err error
	tx.hash, err = tx.computeHash(f.hashFactory)
	if err != nil {
		return nil, xerrors.Errorf("couldn't hash the tx: %v", err)
	}

	return tx, nil
}

func (f TransactionFactory) FromText(text []byte) (ledger.Transaction, error) {
	tx, err := newTransaction(f.hashFactory, string(text))
	if err != nil {
		return nil, err
	}

	return tx, nil
}
