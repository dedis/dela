package consumer

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/ledger"
	"go.dedis.ch/fabric/ledger/inventory"
)

// Output is the result of a transaction execution.
type Output struct {
	Key      []byte
	Instance proto.Message
}

// Consumer is an abstraction for a ledger to consume the incoming transactions.
// It is responsible for processing the transactions and producing the instances
// that will later be stored in the inventory.
type Consumer interface {
	GetTransactionFactory() ledger.TransactionFactory

	Consume(tx ledger.Transaction, page inventory.Page) (Output, error)
}
