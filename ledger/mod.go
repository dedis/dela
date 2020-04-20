package ledger

import (
	"context"

	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/mino"
)

// Actor provides the primitives to send transactions to the public ledger.
type Actor interface {
	// Setup is the function to call to initialize a ledger from scratch. It
	// takes the roster for the initial list of players and returns nil if the
	// ledger can be created from it, otherwise it returns an error.
	Setup(roster mino.Players) error

	// AddTransaction spreads the transaction so that it will be included in the
	// next blocks.
	AddTransaction(tx consumer.Transaction) error
}

// TransactionResult is the result of a transaction execution.
type TransactionResult interface {
	GetTransactionID() []byte
}

// Ledger provides the primitives to update a distributed public ledger through
// transactions.
type Ledger interface {
	Listen() (Actor, error)

	// GetInstance returns the instance of the key if it exists, otherwise an
	// error.
	// TODO: verifiable instance.
	GetInstance(key []byte) (consumer.Instance, error)

	// Watch populates the channel with new incoming transaction results.
	Watch(ctx context.Context) <-chan TransactionResult
}
