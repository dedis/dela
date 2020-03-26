package byzcoin

import (
	fmt "fmt"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/ledger"
	"golang.org/x/xerrors"
)

// Digest is the type of a transaction ID.
//
// - implements fmt.Stringer
type Digest [32]byte

// String implements fmt.Stringer. It returns a string representation of the ID.
func (d Digest) String() string {
	return fmt.Sprintf("0x%x", d[:])[:16]
}

// Transaction is an atomic execution.
//
// - implements ledger.Transaction
// - implements encoding.Packable
// - implements fmt.Stringer
type Transaction struct {
	hash Digest
	// TODO: smart contract input
	value string
}

func newTransaction(f crypto.HashFactory, value string) (Transaction, error) {
	tx := Transaction{value: value}

	var err error
	tx.hash, err = tx.computeHash(f)
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

func (t Transaction) String() string {
	return fmt.Sprintf("Transaction{Value: %s}", t.value)
}

func (t Transaction) computeHash(f crypto.HashFactory) (Digest, error) {
	h := f.New()
	_, err := h.Write([]byte(t.value))
	if err != nil {
		return Digest{}, xerrors.Errorf("couldn't write t.value: %v", err)
	}

	d := Digest{}
	copy(d[:], h.Sum(nil))
	return d, nil
}

// TransactionResult contains the result of a transaction execution.
//
// - implements ledger.TransactionResult
// - implements fmt.Stringer
type TransactionResult struct {
	txID Digest
}

// GetTransactionID implements ledger.TransactionResult. It returns the unique
// identifier of the related transaction.
func (tr TransactionResult) GetTransactionID() []byte {
	return tr.txID[:]
}

// String implements fmt.Stringer. It returns a string representation of the
// transaction result.
func (tr TransactionResult) String() string {
	return fmt.Sprintf("TransactionResult@%v", tr.txID)
}

// transactionFactory is an implementation of a Byzcoin transaction factory.
//
// - implements ledger.TransactionFactory
type transactionFactory struct {
	hashFactory crypto.HashFactory
}

func newTransactionFactory() transactionFactory {
	return transactionFactory{
		hashFactory: crypto.NewSha256Factory(),
	}
}

// Create implements ledger.TransactionFactory. It returns a new transaction
// built from the arguments.
func (f transactionFactory) Create(args ...interface{}) (ledger.Transaction, error) {
	tx, err := newTransaction(f.hashFactory, args[0].(string))
	if err != nil {
		return nil, xerrors.Errorf("couldn't make tx: %v", err)
	}

	return tx, nil
}

// FromProto implements ledger.TransactionFactory. It returns a new transaction
// built from the protobuf message.
func (f transactionFactory) FromProto(pb proto.Message) (ledger.Transaction, error) {
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
