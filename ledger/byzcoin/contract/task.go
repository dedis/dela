package contract

import (
	"fmt"
	"io"

	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

// SpawnTask is a client task of a transaction to create a new instance.
//
// - implements basic.ClientTask
type SpawnTask struct {
	ContractID string
	Argument   map[string]string
}

// Fingerprint implements encoding.Fingerprinter. It serializes the task into
// the writer in a deterministic way.
func (act SpawnTask) Fingerprint(w io.Writer) error {
	_, err := w.Write([]byte(act.ContractID))
	if err != nil {
		return xerrors.Errorf("couldn't write contract: %v", err)
	}

	for k, v := range act.Argument {
		_, err = w.Write([]byte(fmt.Sprintf("%s:%s", k, v)))
		if err != nil {
			return xerrors.Errorf("couldn't write argument: %v", err)
		}
	}

	return nil
}

func (t SpawnTask) Serialize(serdeng.Context) ([]byte, error) {
	return nil, xerrors.New("not implemented")
}

// InvokeTask is a client task of a transaction to update an existing instance
// if the access rights control allows it.
//
// - implements basic.ClientTask
type InvokeTask struct {
	Key      []byte
	Argument map[string]string
}

// Fingerprint implements encoding.Fingeprinter. It serializes the task into the
// writer in a deterministic way.
func (act InvokeTask) Fingerprint(w io.Writer) error {
	_, err := w.Write(act.Key)
	if err != nil {
		return xerrors.Errorf("couldn't write key: %v", err)
	}

	for k, v := range act.Argument {
		_, err = w.Write([]byte(fmt.Sprintf("%s:%s", k, v)))
		if err != nil {
			return xerrors.Errorf("couldn't write argument: %v", err)
		}
	}

	return nil
}

func (t InvokeTask) Serialize(serdeng.Context) ([]byte, error) {
	return nil, xerrors.New("not implemented")
}

// DeleteTask is a client task of a transaction to mark an instance as deleted
// so that it cannot be updated anymore.
//
// - implements basic.ClientTask
type DeleteTask struct {
	Key []byte
}

// Fingerprint implements encoding.Fingerprinter. It serializes the task into
// the writer in a deterministic way.
func (a DeleteTask) Fingerprint(w io.Writer) error {
	_, err := w.Write(a.Key)
	if err != nil {
		return xerrors.Errorf("couldn't write key: %v", err)
	}

	return nil
}

func (t DeleteTask) Serialize(serdeng.Context) ([]byte, error) {
	return nil, xerrors.New("not implemented")
}

// serverTask is a contract task that can be consumed to update an inventory
// page.
//
// - implements basic.ServerTask
type serverTask struct {
	basic.ClientTask

	contracts map[string]Contract
}

// Consume implements basic.ServerTask. It updates the page according to the
// task definition.
func (act serverTask) Consume(ctx basic.Context, page inventory.WritablePage) error {
	txCtx := taskContext{
		Context: ctx,
		page:    page,
	}

	var instance *Instance
	var err error
	switch task := act.ClientTask.(type) {
	case SpawnTask:
		instance, err = act.consumeSpawn(SpawnContext{
			Context:   txCtx,
			SpawnTask: task,
		})
	case InvokeTask:
		instance, err = act.consumeInvoke(InvokeContext{
			Context:    txCtx,
			InvokeTask: task,
		})
	case DeleteTask:
		instance, err = act.consumeDelete(DeleteContext{
			Context:    txCtx,
			DeleteTask: task,
		})
	default:
		return xerrors.Errorf("invalid task type '%T'", act.ClientTask)
	}

	if err != nil {
		// No wrapping to avoid redundancy in the error message.
		return err
	}

	err = page.Write(instance.Key, instance)
	if err != nil {
		return xerrors.Errorf("couldn't write instance to page: %v", err)
	}

	return nil
}

func (act serverTask) consumeSpawn(ctx SpawnContext) (*Instance, error) {
	_, err := ctx.Read(ctx.GetID())
	if err == nil {
		return nil, xerrors.New("instance already exists")
	}

	exec := act.contracts[ctx.ContractID]
	if exec == nil {
		return nil, xerrors.Errorf("contract '%s' not found", ctx.ContractID)
	}

	value, arcid, err := exec.Spawn(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't execute spawn: %v", err)
	}

	rule := arc.Compile(ctx.ContractID, "spawn")

	err = act.hasAccess(ctx, arcid, rule)
	if err != nil {
		return nil, xerrors.Errorf("no access: %v", err)
	}

	instance := &Instance{
		Key:           ctx.GetID(),
		AccessControl: arcid,
		ContractID:    ctx.ContractID,
		Deleted:       false,
		Value:         value,
	}

	return instance, nil
}

func (act serverTask) consumeInvoke(ctx InvokeContext) (*Instance, error) {
	instance, err := ctx.Read(ctx.Key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the instance: %v", err)
	}

	rule := arc.Compile(instance.ContractID, "invoke")

	err = act.hasAccess(ctx, instance.AccessControl, rule)
	if err != nil {
		return nil, xerrors.Errorf("no access: %v", err)
	}

	exec := act.contracts[instance.ContractID]
	if exec == nil {
		return nil, xerrors.Errorf("contract '%s' not found", instance.ContractID)
	}

	value, err := exec.Invoke(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't invoke: %v", err)
	}

	instance.Value = value

	return instance, nil
}

func (act serverTask) consumeDelete(ctx DeleteContext) (*Instance, error) {
	instance, err := ctx.Read(ctx.Key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the instance: %v", err)
	}

	instance.Deleted = true

	return instance, nil
}

func (act serverTask) hasAccess(ctx Context, key []byte, rule string) error {
	access, err := ctx.GetArc(key)
	if err != nil {
		return xerrors.Errorf("couldn't read access: %v", err)
	}

	err = access.Match(rule, ctx.GetIdentity())
	if err != nil {
		return xerrors.Errorf("%v is refused to '%s' by %v: %v",
			ctx.GetIdentity(), rule, access, err)
	}

	return nil
}

// TaskFactory is a factory to decode protobuf messages into transaction tasks
// and register static contracts.
//
// - implements basic.TaskFactory
type TaskFactory struct {
	contracts  map[string]Contract
	arcFactory serde.Factory
}

// NewTaskFactory returns a new empty instance of the factory.
func NewTaskFactory() TaskFactory {
	return TaskFactory{
		contracts:  make(map[string]Contract),
		arcFactory: serde.UnimplementedFactory{},
	}
}

// Register registers the contract using the name as the identifier. If an
// identifier already exists, it will be overwritten.
func (f TaskFactory) Register(name string, contract Contract) {
	f.contracts[name] = contract
}
