// Package txn contains the definition of a transaction and its factory.
package txn

import (
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/serde"
)

// Transaction is the definition of an action that should be applied to the
// global state.
type Transaction interface {
	serde.Message
	serde.Fingerprinter

	// GetID returns the unique identifier for the transaction.
	GetID() []byte

	// GetIdentity returns the identity that created the transaction.
	GetIdentity() access.Identity

	// GetArg is a getter for the arguments of the transaction.
	GetArg(key string) []byte
}

// Factory is the definition of a factory to deserialize transaction
// messages.
type Factory interface {
	serde.Factory

	TransactionOf(serde.Context, []byte) (Transaction, error)
}

// Arg is a generic argument that can be stored in a transaction.
type Arg struct {
	Key   string
	Value []byte
}

// Manager is a manager to create transaction. It can help creating
// transactions when some information is required like the current nonce.
type Manager interface {
	Make(args ...Arg) (Transaction, error)
}
