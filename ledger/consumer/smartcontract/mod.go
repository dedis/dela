package smartcontract

import (
	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Contract is an interface that provides the primitives to execute a smart
// contract transaction and produce the resulting instance.
type Contract interface {
	// Spawn is called to create a new instance.
	Spawn(ctx SpawnContext) (proto.Message, error)

	// Invoke is called to update an existing instance.
	Invoke(ctx InvokeContext) (proto.Message, error)
}

// Consumer is a consumer of smart contract transactions.
//
// - implements consumer.Consumer
type Consumer struct {
	encoder   encoding.ProtoMarshaler
	contracts map[string]Contract
}

// NewConsumer returns a new instance of the smart contract consumer.
func NewConsumer() Consumer {
	return Consumer{
		encoder:   encoding.NewProtoEncoder(),
		contracts: make(map[string]Contract),
	}
}

// Register registers an executor that can be triggered by a transaction with
// the contract ID sets to the provided name.
func (c Consumer) Register(name string, exec Contract) {
	c.contracts[name] = exec
}

// GetTransactionFactory implements consumer.Consumer. It returns the factory
// for smart contract transactions.
func (c Consumer) GetTransactionFactory() consumer.TransactionFactory {
	return NewTransactionFactory()
}

// GetInstanceFactory implements consumer.Consumer. It returns the factory for
// smart contract instances.
func (c Consumer) GetInstanceFactory() consumer.InstanceFactory {
	return instanceFactory{encoder: c.encoder}
}

// Consume implements consumer.Consumer. It returns the instance produced from
// the execution of the transaction.
func (c Consumer) Consume(in consumer.Transaction,
	page inventory.Page) (consumer.Instance, error) {

	switch tx := in.(type) {
	case SpawnTransaction:
		ctx := SpawnContext{
			transactionContext: transactionContext{
				encoder: c.encoder,
				page:    page,
			},
			Transaction: tx,
		}

		return c.consumeSpawn(ctx)
	case InvokeTransaction:
		ctx := InvokeContext{
			transactionContext: transactionContext{
				encoder: c.encoder,
				page:    page,
			},
			Transaction: tx,
		}

		return c.consumeInvoke(ctx)
	case DeleteTransaction:
		ctx := transactionContext{
			encoder: c.encoder,
			page:    page,
		}

		instance, err := ctx.Read(tx.Key)
		if err != nil {
			return nil, xerrors.Errorf("couldn't read the instance: %v", err)
		}

		ci := instance.(contractInstance)
		ci.deleted = true

		return ci, nil
	default:
		return nil, xerrors.Errorf("invalid tx type '%T'", in)
	}
}

func (c Consumer) consumeSpawn(ctx SpawnContext) (consumer.Instance, error) {
	contractID := ctx.Transaction.ContractID

	exec := c.contracts[contractID]
	if exec == nil {
		return nil, xerrors.Errorf("unknown contract with id '%s'", contractID)
	}

	value, err := exec.Spawn(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't execute spawn: %v", err)
	}

	instance := contractInstance{
		key:        ctx.Transaction.hash,
		contractID: contractID,
		deleted:    false,
		value:      value,
	}

	return instance, nil
}

func (c Consumer) consumeInvoke(ctx InvokeContext) (consumer.Instance, error) {
	inst, err := ctx.Read(ctx.Transaction.Key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the instance: %v", err)
	}

	contractID := inst.GetContractID()

	exec := c.contracts[contractID]
	if exec == nil {
		return nil, xerrors.Errorf("unknown contract with id '%s'", contractID)
	}

	ci := inst.(contractInstance)
	ci.value, err = exec.Invoke(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't invoke: %v", err)
	}

	return ci, nil
}
