package tap

import (
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/serde"
)

// Transaction is the definition of an action that should be applied to the
// global state.
type Transaction interface {
	serde.Fingerprinter

	GetID() []byte

	GetIdentity() arc.Identity

	GetArg(key string) []byte
}
