package smartcontract

import (
	"go.dedis.ch/fabric/ledger"
	"go.dedis.ch/fabric/ledger/inventory"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Consumer is a consumer of smart contract transactions.
type Consumer struct{}

// NewConsumer returns a new instance of the smart contract consumer.
func NewConsumer() Consumer {
	return Consumer{}
}

func (c Consumer) GetTransactionFactory() ledger.TransactionFactory {
	return NewTransactionFactory()
}

// Consume returns the instances produce from the execution of the transaction.
func (c Consumer) Consume(tx ledger.Transaction) ([]inventory.Instance, error) {
	return nil, nil
}
