package byzcoin

import (
	"crypto/sha256"
	fmt "fmt"
	"hash"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/ledger"
	"golang.org/x/xerrors"
)

// Digest is the type of a transaction ID.
type Digest [32]byte

func (d Digest) String() string {
	return fmt.Sprintf("0x%x", d[:])[:16]
}

// Transaction is an atomic execution.
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

// GetID returns the unique identifier of the transaction.
func (t Transaction) GetID() []byte {
	return t.hash[:]
}

// Pack returns the protobuf message of a transaction.
func (t Transaction) Pack() (proto.Message, error) {
	return &TransactionProto{Value: t.value}, nil
}

func (t Transaction) computeHash(f crypto.HashFactory) (Digest, error) {
	h := f.New()
	h.Write([]byte(t.value))

	d := Digest{}
	copy(d[:], h.Sum(nil))
	return d, nil
}

// TransactionResult is a summary of the result of a transaction execution.
type TransactionResult struct {
	txID Digest
}

// GetTransactionID returns the unique identifier of the related transaction.
func (tr TransactionResult) GetTransactionID() []byte {
	return tr.txID[:]
}

func (tr TransactionResult) String() string {
	return fmt.Sprintf("TransactionResult: %v", tr.txID)
}

type sha256Factory struct{}

func (h sha256Factory) New() hash.Hash {
	return sha256.New()
}

type transactionFactory struct {
	hashFactory sha256Factory
}

func (f transactionFactory) Create(args ...interface{}) (ledger.Transaction, error) {
	tx, err := newTransaction(f.hashFactory, args[0].(string))
	if err != nil {
		return nil, xerrors.Errorf("couldn't make tx: %v", err)
	}

	return tx, nil
}

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
