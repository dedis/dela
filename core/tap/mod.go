package tap

import (
	"go.dedis.ch/dela/ledger/arc"
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
	GetIdentity() arc.Identity

	// GetArg is a getter for the arguments of the transaction.
	GetArg(key string) []byte
}

type TransactionFactory interface {
	serde.Factory

	TransactionOf(serde.Context, []byte) (Transaction, error)
}
