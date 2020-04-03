package smartcontract

import (
	"bytes"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/arc/common"
	"go.dedis.ch/fabric/ledger/consumer"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Contract is an interface that provides the primitives to execute a smart
// contract transaction and produce the resulting instance.
type Contract interface {
	// Spawn is called to create a new instance. It returns the initial value of
	// the new instance and its access control ID.
	Spawn(ctx SpawnContext) (proto.Message, []byte, error)

	// Invoke is called to update an existing instance.
	Invoke(ctx InvokeContext) (proto.Message, error)
}

// Consumer is a consumer of smart contract transactions.
//
// - implements consumer.Consumer
type Consumer struct {
	encoder   encoding.ProtoMarshaler
	contracts map[string]Contract

	AccessFactory arc.AccessControlFactory
}

// NewConsumer returns a new instance of the smart contract consumer.
func NewConsumer() Consumer {
	return Consumer{
		encoder:       encoding.NewProtoEncoder(),
		contracts:     make(map[string]Contract),
		AccessFactory: common.NewAccessControlFactory(),
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
		ctx := SpawnContext{Context: ctx, Action: action}

		return c.consumeSpawn(ctx)
	case InvokeAction:
		ctx := InvokeContext{Context: ctx, Action: action}

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

	value, arcid, err := exec.Spawn(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't execute spawn: %v", err)
	}

	if !bytes.Equal(arcid, ctx.GetTransaction().GetID()) {
		// If the instance is a new access control, it is left to the contract
		// to insure the transaction is correct.

		rule := arc.Compile(ctx.GetAction().ContractID, "spawn")

		err = c.hasAccess(ctx, arcid, rule)
		if err != nil {
			return nil, xerrors.Errorf("no access: %v", err)
		}
	}

	instance := contractInstance{
		key:           ctx.GetTransaction().GetID(),
		accessControl: arcid,
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

	rule := arc.Compile(contractID, "invoke")

	err = c.hasAccess(ctx, inst.GetArcID(), rule)
	if err != nil {
		return nil, xerrors.Errorf("no access: %v", err)
	}

	ci := inst.(contractInstance)
	ci.value, err = exec.Invoke(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't invoke: %v", err)
	}

	return ci, nil
}

func (c Consumer) hasAccess(ctx consumer.Context, key []byte, rule string) error {
	instance, err := ctx.Read(key)
	if err != nil {
		return err
	}

	access, err := c.AccessFactory.FromProto(instance.GetValue())
	if err != nil {
		return err
	}

	err = access.Match(rule, ctx.GetTransaction().GetIdentity())
	if err != nil {
		return xerrors.Errorf("%v is refused to '%s' by %v: %v",
			ctx.GetTransaction().GetIdentity(), rule, access, err)
	}

	return nil
}
