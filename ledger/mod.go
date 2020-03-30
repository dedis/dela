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

	// GetID returns a unique identifier for the transaction.
	GetID() []byte
}

// TransactionFactory is a factory to create new transactions or decode from
// network messages.
type TransactionFactory interface {
	FromProto(pb proto.Message) (Transaction, error)
}

// Actor provides the primitives to send transactions to the public ledger.
type Actor interface {
	AddTransaction(tx Transaction) error
}

// TransactionResult is the result of a transaction execution.
type TransactionResult interface {
	GetTransactionID() []byte
}

// Ledger provides the primitives to update a distributed public ledger through
// transactions.
type Ledger interface {
	Listen(mino.Players) (Actor, error)

	Watch(ctx context.Context) <-chan TransactionResult
}
