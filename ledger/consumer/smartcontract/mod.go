package smartcontract

import (
	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger"
	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Executor is an interface that provides the primitives to execute a smart
// contract and produce the instances of a transaction.
type Executor interface {
	Spawn(ctx SpawnContext) (proto.Message, error)
	Invoke(ctx InvokeContext) (proto.Message, error)
}

// Consumer is a consumer of smart contract transactions.
type Consumer struct {
	encoder   encoding.ProtoMarshaler
	executors map[string]Executor
}

// NewConsumer returns a new instance of the smart contract consumer.
func NewConsumer() Consumer {
	return Consumer{
		encoder:   encoding.NewProtoEncoder(),
		executors: make(map[string]Executor),
	}
}

// Register registers an executor that can be triggers by a transaction with the
// contract ID sets to the name.
func (c Consumer) Register(name string, exec Executor) {
	c.executors[name] = exec
}

// GetTransactionFactory returns the factory for smart contract transactions.
func (c Consumer) GetTransactionFactory() ledger.TransactionFactory {
	return NewTransactionFactory()
}

// Consume returns the instances produce from the execution of the transaction.
func (c Consumer) Consume(in ledger.Transaction, page inventory.Page) (consumer.Output, error) {
	out := consumer.Output{}

	switch tx := in.(type) {
	case SpawnTransaction:
		ctx := SpawnContext{
			transactionContext: transactionContext{
				encoder: c.encoder,
				page:    page,
			},
			Transaction: tx,
		}

		out, err := c.consumeSpawn(ctx)
		if err != nil {
			return out, err
		}

		return out, nil
	case InvokeTransaction:
		ctx := InvokeContext{
			transactionContext: transactionContext{
				encoder: c.encoder,
				page:    page,
			},
			Transaction: tx,
		}

		out, err := c.consumeInvoke(ctx)
		if err != nil {
			return out, err
		}

		return out, nil
	case DeleteTransaction:
		out := consumer.Output{}

		instancepb, err := page.Read(tx.Key)
		if err != nil {
			return out, xerrors.Errorf("couldn't read the instance: %v", err)
		}

		instance := instancepb.(*InstanceProto)
		instance.Deleted = true

		out.Key = tx.Key
		out.Instance = instance

		return out, nil
	default:
		return out, xerrors.Errorf("invalid tx type '%T'", in)
	}
}

func (c Consumer) consumeSpawn(ctx SpawnContext) (out consumer.Output, err error) {
	contractID := ctx.Transaction.ContractID

	exec := c.executors[contractID]
	if exec == nil {
		err = xerrors.Errorf("unknown contract with id '%s'", contractID)
		return
	}

	value, err := exec.Spawn(ctx)
	if err != nil {
		err = xerrors.Errorf("couldn't execute spawn: %v", err)
		return
	}

	instance := instance{
		contractID: contractID,
		deleted:    false,
		value:      value,
	}

	out.Instance, err = c.packInstance(instance)
	if err != nil {
		return
	}

	out.Key = ctx.Transaction.hash
	return
}

func (c Consumer) consumeInvoke(ctx InvokeContext) (out consumer.Output, err error) {
	inst, err := ctx.Read(ctx.Transaction.Key)
	if err != nil {
		err = xerrors.Errorf("couldn't read the instance: %v", err)
		return
	}

	contractID := inst.GetContractID()

	exec := c.executors[contractID]
	if exec == nil {
		err = xerrors.Errorf("unknown contract with id '%s'", contractID)
		return
	}

	value, err := exec.Invoke(ctx)
	if err != nil {
		err = xerrors.Errorf("couldn't invoke: %v", err)
		return
	}

	instance := instance{
		contractID: contractID,
		deleted:    inst.IsDeleted(),
		value:      value,
	}

	out.Instance, err = c.packInstance(instance)
	if err != nil {
		return
	}

	out.Key = ctx.Transaction.Key

	return
}

func (c Consumer) packInstance(instance Instance) (proto.Message, error) {
	pb := &InstanceProto{
		ContractID: instance.GetContractID(),
		Deleted:    instance.IsDeleted(),
	}

	var err error
	pb.Value, err = c.encoder.MarshalAny(instance.GetValue())
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal the value: %v", err)
	}

	return pb, nil
}
