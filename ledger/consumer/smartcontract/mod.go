package smartcontract

import (
	fmt "fmt"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/ledger/permissions"
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

	// TODO: common factory
	AccessFactory permissions.AccessControlFactory
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
	return NewTransactionFactory(nil)
}

// GetInstanceFactory implements consumer.Consumer. It returns the factory for
// smart contract instances.
func (c Consumer) GetInstanceFactory() consumer.InstanceFactory {
	return instanceFactory{encoder: c.encoder}
}

// Consume implements consumer.Consumer. It returns the instance produced from
// the execution of the transaction.
func (c Consumer) Consume(ctx consumer.Context) (consumer.Instance, error) {
	tx, ok := ctx.GetTransaction().(transaction)
	if !ok {
		return nil, xerrors.Errorf("invalid tx type '%T'", ctx.GetTransaction())
	}

	switch action := tx.action.(type) {
	case SpawnAction:
		ctx := SpawnContext{Context: ctx, action: action}

		return c.consumeSpawn(ctx)
	case InvokeAction:
		ctx := InvokeContext{Context: ctx, action: action}

		return c.consumeInvoke(ctx)
	case DeleteAction:
		instance, err := ctx.Read(action.Key)
		if err != nil {
			return nil, xerrors.Errorf("couldn't read the instance: %v", err)
		}

		ci := instance.(contractInstance)
		ci.deleted = true

		return ci, nil
	default:
		return nil, xerrors.Errorf("invalid action type '%T'", action)
	}
}

func (c Consumer) consumeSpawn(ctx SpawnContext) (consumer.Instance, error) {
	contractID := ctx.GetAction().ContractID

	exec := c.contracts[contractID]
	if exec == nil {
		return nil, xerrors.Errorf("unknown contract with id '%s'", contractID)
	}

	value, err := exec.Spawn(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't execute spawn: %v", err)
	}

	instance := contractInstance{
		key:           ctx.GetTransaction().GetID(),
		accessControl: []byte{},
		contractID:    contractID,
		deleted:       false,
		value:         value,
	}

	return instance, nil
}

func (c Consumer) consumeInvoke(ctx InvokeContext) (consumer.Instance, error) {
	inst, err := ctx.Read(ctx.GetAction().Key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the instance: %v", err)
	}

	contractID := inst.(ContractInstance).GetContractID()

	exec := c.contracts[contractID]
	if exec == nil {
		return nil, xerrors.Errorf("unknown contract with id '%s'", contractID)
	}

	access, err := c.getAccessControl(ctx, inst.GetAccessControlID())
	if err != nil {
		return nil, xerrors.Errorf("couldn't read access control: %v", err)
	}

	// TODO: improve rule
	rule := fmt.Sprintf("invoke:%s", contractID)

	if !access.Match(ctx.GetTransaction().GetIdentity(), rule) {
		return nil, xerrors.Errorf("%v is refused to '%s' by %v",
			ctx.GetTransaction().GetIdentity(), rule, access)
	}

	ci := inst.(contractInstance)
	ci.value, err = exec.Invoke(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't invoke: %v", err)
	}

	return ci, nil
}

func (c Consumer) getAccessControl(ctx consumer.Context, key []byte) (permissions.AccessControl, error) {
	instance, err := ctx.Read(key)
	if err != nil {
		return nil, err
	}

	access, err := c.AccessFactory.FromProto(instance.GetValue())
	if err != nil {
		return nil, err
	}

	return access, nil
}
