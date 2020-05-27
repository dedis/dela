package transactions

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/ledger/inventory"
)

// ClientTransaction is a transaction created by a client that will be sent to
// the network.
type ClientTransaction interface {
	encoding.Packable

	// GetID returns a unique identifier for the transaction.
	GetID() []byte
}

// ServerTransaction is an extension of the client transaction that will be
// consumed by the server.
type ServerTransaction interface {
	ClientTransaction

	Consume(inventory.WritablePage) error
}

// TransactionFactory is a factory to create new transactions or decode from
// network messages.
type TransactionFactory interface {
	// FromProto returns the transaction from the protobuf message.
	FromProto(pb proto.Message) (ServerTransaction, error)
}
