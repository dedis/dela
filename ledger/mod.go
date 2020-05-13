package ledger

import (
	"context"

	"go.dedis.ch/fabric/ledger/transactions"
	"go.dedis.ch/fabric/mino"
)

// Actor provides the primitives to send transactions to the public ledger.
type Actor interface {
	// HasStarted returns a channel that will be populated with errors that
	// occurred while the ledger was initializing, or it will be closed if it
	// succeeded.
	HasStarted() <-chan error

	// Setup is the function to call to initialize a ledger from scratch. It
	// takes the roster for the initial list of players and returns nil if the
	// ledger can be created from it, otherwise it returns an error.
	Setup(roster mino.Players) error

	// AddTransaction spreads the transaction so that it will be included in the
	// next blocks.
	AddTransaction(tx transactions.ClientTransaction) error

	// Close stops the ledger and cleans the states.
	Close() error
}

// TransactionResult is the result of a transaction execution.
type TransactionResult interface {
	GetTransactionID() []byte
}

// Ledger provides the primitives to update a distributed public ledger through
// transactions.
type Ledger interface {
	Listen() (Actor, error)

	// Watch populates the channel with new incoming transaction results.
	Watch(ctx context.Context) <-chan TransactionResult
}
