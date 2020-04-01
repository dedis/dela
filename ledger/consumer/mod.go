package consumer

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/permissions"
)

// Transaction is an atomic execution of one or several instructions.
type Transaction interface {
	encoding.Packable

	// GetID returns a unique identifier for the transaction.
	GetID() []byte

	GetIdentity() permissions.Identity
}

// TransactionFactory is a factory to create new transactions or decode from
// network messages.
type TransactionFactory interface {
	// FromProto returns the transaction from the protobuf message.
	FromProto(pb proto.Message) (Transaction, error)
}

// Instance is the result of a transaction execution.
type Instance interface {
	encoding.Packable

	// GetKey returns the identifier of the instance.
	GetKey() []byte

	GetAccessControlID() []byte

	// GetValue returns the value stored in the instance.
	GetValue() proto.Message
}

// InstanceFactory is an abstraction to decode protobuf messages into instances.
type InstanceFactory interface {
	// FromProto returns the instance from the protobuf message.
	FromProto(pb proto.Message) (Instance, error)
}

type Context interface {
	GetTransaction() Transaction

	Read([]byte) (Instance, error)
}

// Consumer is an abstraction for a ledger to consume the incoming transactions.
// It is responsible for processing the transactions and producing the instances
// that will later be stored in the inventory.
type Consumer interface {
	// GetTransactionFactory returns the transaction factory.
	GetTransactionFactory() TransactionFactory

	// GetInstanceFactory returns the instance factory.
	GetInstanceFactory() InstanceFactory

	// Consume returns the resulting instance of the transaction execution. The
	// current page of the inventory is provided.
	Consume(ctx Context) (Instance, error)
}
