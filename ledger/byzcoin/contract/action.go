package contract

import (
	"io"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/arc/common"
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
func (act SpawnAction) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	argument, err := enc.MarshalAny(act.Argument)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack argument: %v", err)
	}

	pb := &SpawnActionProto{
		ContractID: act.ContractID,
		Argument:   argument,
	}

	return pb, nil
}

// Fingerprint implements encoding.Fingerprinter.
func (act SpawnAction) Fingerprint(w io.Writer, e encoding.ProtoMarshaler) error {
	_, err := w.Write([]byte(act.ContractID))
	if err != nil {
		return xerrors.Errorf("couldn't write contract: %v", err)
	}

	err = e.MarshalStable(w, act.Argument)
	if err != nil {
		return xerrors.Errorf("couldn't write argument: %v", err)
	}

	return nil
}

// InvokeAction is a contract transaction action to update an existing instance
// of the access control allows it.
type InvokeAction struct {
	Key      []byte
	Argument proto.Message
}

// Pack implements encoding.Packable.
func (act InvokeAction) Pack(e encoding.ProtoMarshaler) (proto.Message, error) {
	argument, err := e.MarshalAny(act.Argument)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack argument: %v", err)
	}

	pb := &InvokeActionProto{
		Key:      act.Key,
		Argument: argument,
	}

	return pb, nil
}

// Fingerprint implements encoding.Fingeprinter.
func (act InvokeAction) Fingerprint(w io.Writer, e encoding.ProtoMarshaler) error {
	_, err := w.Write(act.Key)
	if err != nil {
		return xerrors.Errorf("couldn't write key: %v", err)
	}

	err = e.MarshalStable(w, act.Argument)
	if err != nil {
		return xerrors.Errorf("couldn't write argument: %v", err)
	}

	return nil
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

// Fingerprint implements encoding.Fingerprinter.
func (a DeleteAction) Fingerprint(w io.Writer, e encoding.ProtoMarshaler) error {
	_, err := w.Write(a.Key)
	if err != nil {
		return xerrors.Errorf("couldn't write key: %v", err)
	}

	return nil
}

type serverAction struct {
	basic.ClientAction
	contracts  map[string]Contract
	arcFactory arc.AccessControlFactory
	encoder    encoding.ProtoMarshaler
}

func (act serverAction) Consume(ctx basic.Context, page inventory.WritablePage) error {
	txCtx := actionContext{
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
		return xerrors.Errorf("invalid action type '%T'", act.ClientAction)
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
		return nil, xerrors.Errorf("couldn't pack value: %v", err)
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
		return nil, xerrors.Errorf("couldn't pack value: %v", err)
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
		return xerrors.Errorf("couldn't read access: %v", err)
	}

	err = access.Match(rule, ctx.GetIdentity())
	if err != nil {
		return xerrors.Errorf("%v is refused to '%s' by %v: %v",
			ctx.GetIdentity(), rule, access, err)
	}

	return nil
}

// ActionFactory is a factory to decode protobuf messages into contract actions
// and register static contracts.
type ActionFactory struct {
	contracts  map[string]Contract
	arcFactory arc.AccessControlFactory
	encoder    encoding.ProtoMarshaler
}

// NewActionFactory returns a new empty instance of the factory.
func NewActionFactory() ActionFactory {
	return ActionFactory{
		contracts:  make(map[string]Contract),
		arcFactory: common.NewAccessControlFactory(),
		encoder:    encoding.NewProtoEncoder(),
	}
}

// Register registers the contract using the name as the identifier. If an
// identifier already exists, it will be overwritten.
func (f ActionFactory) Register(name string, contract Contract) {
	f.contracts[name] = contract
}

// FromProto implements basic.ActionFactory. It returns the server action of a
// protobuf message when appropriate, otherwise an error.
func (f ActionFactory) FromProto(in proto.Message) (basic.ServerAction, error) {
	inAny, ok := in.(*any.Any)
	if ok {
		var err error
		in, err = f.encoder.UnmarshalDynamicAny(inAny)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal message: %v", err)
		}
	}

	action := serverAction{
		contracts:  f.contracts,
		arcFactory: f.arcFactory,
		encoder:    f.encoder,
	}

	switch pb := in.(type) {
	case *SpawnActionProto:
		action.ClientAction = SpawnAction{
			ContractID: pb.GetContractID(),
			Argument:   pb.GetArgument(),
		}
	case *InvokeActionProto:
		action.ClientAction = InvokeAction{
			Key:      pb.GetKey(),
			Argument: pb.GetArgument(),
		}
	case *DeleteActionProto:
		action.ClientAction = DeleteAction{
			Key: pb.GetKey(),
		}
	default:
		return nil, xerrors.Errorf("invalid message type '%T'", in)
	}

	return action, nil
}
