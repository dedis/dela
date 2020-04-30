package contract

import (
	"io"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/inventory"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"golang.org/x/xerrors"
)

// SpawnAction is a contract transaction action to create a new instance.
type SpawnAction struct {
	ContractID string
	Argument   proto.Message
}

// Pack implements encoding.Packable.
func (act SpawnAction) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &SpawnActionProto{}, nil
}

// WriteTo implements io.WriterTo.
func (act SpawnAction) WriteTo(io.Writer) (int64, error) {
	return 0, nil
}

// InvokeAction is a contract transaction action to update an existing instance
// of the access control allows it.
type InvokeAction struct {
	Key      []byte
	Argument proto.Message
}

// Pack implements encoding.Packable.
func (act InvokeAction) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &SpawnActionProto{}, nil
}

// WriteTo implements io.WriterTo.
func (act InvokeAction) WriteTo(io.Writer) (int64, error) {
	return 0, nil
}

// DeleteAction is a contract transaction action to mark an instance as deleted
// so that it cannot be updated anymore.
type DeleteAction struct {
	Key []byte
}

// Pack implements encoding.Packable.
func (a DeleteAction) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &DeleteActionProto{Key: a.Key}, nil
}

// WriteTo implements io.WriterTo.
func (a DeleteAction) WriteTo(io.Writer) (int64, error) {
	return 0, nil
}

type serverAction struct {
	basic.ClientAction
	contracts  map[string]Contract
	arcFactory arc.AccessControlFactory
	encoder    encoding.ProtoMarshaler
}

func (act serverAction) Consume(ctx basic.Context, page inventory.WritablePage) error {
	txCtx := transactionContext{
		Context:    ctx,
		arcFactory: act.arcFactory,
		page:       page,
	}

	var instance *Instance
	var err error
	switch action := act.ClientAction.(type) {
	case SpawnAction:
		instance, err = act.consumeSpawn(SpawnContext{
			Context:     txCtx,
			SpawnAction: action,
		})
	case InvokeAction:
		instance, err = act.consumeInvoke(InvokeContext{
			Context:      txCtx,
			InvokeAction: action,
		})
	case DeleteAction:
		instance, err = act.consumeDelete(DeleteContext{
			Context:      txCtx,
			DeleteAction: action,
		})
	default:
		return xerrors.New("missing action")
	}

	if err != nil {
		return err
	}

	err = page.Write(instance.Key, instance)
	if err != nil {
		return err
	}

	return nil
}

func (act serverAction) consumeSpawn(ctx SpawnContext) (*Instance, error) {
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

	valueAny, err := act.encoder.MarshalAny(value)
	if err != nil {
		return nil, err
	}

	instance := &Instance{
		Key:           ctx.GetID(),
		AccessControl: arcid,
		ContractID:    ctx.ContractID,
		Deleted:       false,
		Value:         valueAny,
	}

	return instance, nil
}

func (act serverAction) consumeInvoke(ctx InvokeContext) (*Instance, error) {
	instance, err := ctx.Read(ctx.Key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the instance: %v", err)
	}

	rule := arc.Compile(instance.GetContractID(), "invoke")

	err = act.hasAccess(ctx, instance.GetAccessControl(), rule)
	if err != nil {
		return nil, xerrors.Errorf("no access: %v", err)
	}

	exec := act.contracts[instance.GetContractID()]
	if exec == nil {
		return nil, xerrors.Errorf("contract '%s' not found", instance.GetContractID())
	}

	value, err := exec.Invoke(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't invoke: %v", err)
	}

	valueAny, err := act.encoder.MarshalAny(value)
	if err != nil {
		return nil, err
	}

	instance.Value = valueAny

	return instance, nil
}

func (act serverAction) consumeDelete(ctx DeleteContext) (*Instance, error) {
	instance, err := ctx.Read(ctx.Key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the instance: %v", err)
	}

	instance.Deleted = true

	return instance, nil
}

func (act serverAction) hasAccess(ctx Context, key []byte, rule string) error {
	access, err := ctx.GetArc(key)
	if err != nil {
		return xerrors.Errorf("couldn't decode access: %v", err)
	}

	err = access.Match(rule, ctx.GetIdentity())
	if err != nil {
		return xerrors.Errorf("%v is refused to '%s' by %v: %v",
			ctx.GetIdentity(), rule, access, err)
	}

	return nil
}

type actionFactory struct{}

func (f actionFactory) FromProto(in proto.Message) (basic.ServerAction, error) {

	return serverAction{}, nil
}
