package consumer

import (
	"go.dedis.ch/fabric/ledger"
	"go.dedis.ch/fabric/ledger/inventory"
)

// Consumer is an abstraction for a ledger to consume the incoming transactions.
// It is responsible for processing the transactions and producing the instances
// that will later be stored in the inventory.
type Consumer interface {
	GetTransactionFactory() ledger.TransactionFactory

	Consume(tx ledger.Transaction) ([]inventory.Instance, error)
}
