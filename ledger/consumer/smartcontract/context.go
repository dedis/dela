package smartcontract

import (
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/ledger/inventory"
	"go.dedis.ch/fabric/ledger/permissions"
	"golang.org/x/xerrors"
)

type transactionContext struct {
	encoder encoding.ProtoMarshaler
	tx      consumer.Transaction
	page    inventory.Page
}

// NewContext returns a new instance of a smart contract transaction context.
func NewContext(tx consumer.Transaction, page inventory.Page) consumer.Context {
	return transactionContext{
		encoder: encoding.NewProtoEncoder(),
		tx:      tx,
		page:    page,
	}
}

func (ctx transactionContext) GetTransaction() consumer.Transaction {
	return ctx.tx
}

func (ctx transactionContext) GetAccessControl(key []byte) (permissions.AccessControl, error) {
	instance, err := ctx.Read(key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read instance: %v", err)
	}

	// TODO: decode access control
	instance.GetValue()

	return nil, nil
}

func (ctx transactionContext) Read(key []byte) (consumer.Instance, error) {
	entry, err := ctx.page.Read(key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the entry: %v", err)
	}

	factory := instanceFactory{encoder: ctx.encoder}

	instance, err := factory.FromProto(entry)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode instance: %v", err)
	}

	return instance, nil
}

// SpawnContext is the context provided to a smart contract execution of a spawn
// transaction.
type SpawnContext struct {
	consumer.Context
}

// GetTransaction returns the transaction casted as a spawn transaction.
func (ctx SpawnContext) GetTransaction() SpawnTransaction {
	return ctx.Context.GetTransaction().(SpawnTransaction)
}

// InvokeContext is the context provided to a smart contract execution of an
// invoke transaction.
type InvokeContext struct {
	consumer.Context
}

// GetTransaction returns the transaction casted as an invoke transaction.
func (ctx InvokeContext) GetTransaction() InvokeTransaction {
	return ctx.Context.GetTransaction().(InvokeTransaction)
}
