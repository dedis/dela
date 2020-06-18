package transactions

import (
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/serdeng"
)

// ClientTransaction is a transaction created by a client that will be sent to
// the network.
type ClientTransaction interface {
	serdeng.Message

	// GetID returns a unique identifier for the transaction.
	GetID() []byte
}

// ServerTransaction is an extension of the client transaction that will be
// consumed by the server.
type ServerTransaction interface {
	ClientTransaction

	Consume(inventory.WritablePage) error
}

type TxFactory interface {
	serdeng.Factory

	TxOf(serdeng.Context, []byte) (ServerTransaction, error)
}
