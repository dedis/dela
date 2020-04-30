package darc

import (
	"io"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/inventory"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"golang.org/x/xerrors"
)

// darcAction is a transaction action to create or update an access right
// control.
type darcAction struct {
	key    []byte
	access Access
}

// NewCreate returns a new action to create a DARC.
func NewCreate() basic.ClientAction {
	return darcAction{}
}

// NewUpdate returns a new action to update a DARC.
func NewUpdate(key []byte) basic.ClientAction {
	return darcAction{key: key}
}

// Pack implements encoding.Packable.
func (act darcAction) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &ActionProto{
		Key: act.key,
	}

	access, err := enc.Pack(act.access)
	if err != nil {
		return nil, err
	}

	pb.Access = access.(*AccessControlProto)

	return pb, nil
}

// WriteTo implements io.WriterTo.
func (act darcAction) WriteTo(w io.Writer) (int64, error) {
	// TODO: write to
	sum := int64(0)

	n, err := w.Write(act.key)
	sum += int64(n)
	if err != nil {
		return sum, err
	}

	return 0, nil
}

type serverArcAction struct {
	encoder encoding.ProtoMarshaler
	darcAction
}

func (act serverArcAction) Consume(ctx basic.Context, page inventory.WritablePage) error {
	accesspb, err := act.encoder.Pack(act.access)
	if err != nil {
		return err
	}

	err = page.Write(ctx.GetID(), accesspb)
	if err != nil {
		return err
	}

	return nil
}

type actionFactory struct {
	darcFactory Factory
}

// NewActionFactory returns a new instance of the action factory.
func NewActionFactory() basic.ActionFactory {
	return actionFactory{
		darcFactory: NewFactory(),
	}
}

func (f actionFactory) FromProto(in proto.Message) (basic.ServerAction, error) {
	var pb *ActionProto
	switch msg := in.(type) {
	case *ActionProto:
		pb = msg
	default:
		return nil, xerrors.Errorf("invalid message type '%T'", in)
	}

	access, err := f.darcFactory.FromProto(pb.GetAccess())
	if err != nil {
		return nil, err
	}

	servAccess := serverArcAction{
		darcAction: darcAction{
			key:    pb.GetKey(),
			access: access.(Access),
		},
		encoder: f.darcFactory.encoder,
	}

	return servAccess, nil
}
