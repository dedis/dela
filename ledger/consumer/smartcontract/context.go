package smartcontract

import (
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

type transactionContext struct {
	encoder encoding.ProtoMarshaler
	tx      consumer.Transaction
	page    inventory.Page
}

// NewContext returns a new instance of a smart contract transaction context.
//
// - implements consumer.Context
func NewContext(tx consumer.Transaction, page inventory.Page) consumer.Context {
	return transactionContext{
		encoder: encoding.NewProtoEncoder(),
		tx:      tx,
		page:    page,
	}
}

// GetTransaction implements consumer.Context. It returns the transaction of the
// context.
func (ctx transactionContext) GetTransaction() consumer.Transaction {
	return ctx.tx
}

// Read implements consumer.Context. It returns the instance stored to the given
// key, or an error if it does not find it.
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
	Action SpawnAction
}

// GetAction returns the transaction casted as a spawn transaction.
func (ctx SpawnContext) GetAction() SpawnAction {
	return ctx.Action
}

// InvokeContext is the context provided to a smart contract execution of an
// invoke transaction.
type InvokeContext struct {
	consumer.Context
	Action InvokeAction
}

// GetAction returns the transaction casted as an invoke transaction.
func (ctx InvokeContext) GetAction() InvokeAction {
	return ctx.Action
}
