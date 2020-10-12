// Package txn defines the abstraction of transactions.
//
// A transaction is a smart contract input. It is uniquely identifiable via a
// digest and it can be sorted with the nonce that acts as a sequence number.
// The transaction is also created by an identity that can be used for access
// control for instance.
//
// The manager helps to create transactions as the nonce needs to be correct for
// the transaction to be valid.
//
// Documentation Last Review: 08.10.2020
//
package txn

import (
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/serde"
)

// Transaction is what triggers a smart contract execution by passing it as part
// of the input.
type Transaction interface {
	serde.Message
	serde.Fingerprinter

	// GetID returns the unique identifier for the transaction.
	GetID() []byte

	// GetNonce returns the nonce of the transaction which corresponds to the
	// sequence number of a unique identity.
	GetNonce() uint64

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

	Sync() error
}
