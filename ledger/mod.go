package ledger

import (
	"context"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
)

// Transaction is an atomic execution of one or several instructions.
type Transaction interface {
	encoding.Packable

	GetID() []byte
}

// TransactionResult is the result of a transaction execution.
type TransactionResult interface {
	GetTransactionID() []byte
}

// TransactionFactory is a factory to create new transactions or decode from
// network messages.
type TransactionFactory interface {
	Create(...interface{}) (Transaction, error)

	FromProto(pb proto.Message) (Transaction, error)
}

// Actor provides the primitives to send transactions to the public ledger.
type Actor interface {
	AddTransaction(tx Transaction) error
}

// Ledger provides the primitives to update a distributed public ledger through
// transactions.
type Ledger interface {
	GetTransactionFactory() TransactionFactory

	Listen(mino.Players) (Actor, error)

	Watch(ctx context.Context) <-chan TransactionResult
}
