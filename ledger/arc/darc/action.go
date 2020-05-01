package darc

import (
	"io"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/inventory"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"golang.org/x/xerrors"
)

const (
	// UpdateAccessRule is the rule to be defined in the DARC to update it.
	UpdateAccessRule = "darc:update"
)

// darcAction is a transaction action to create or update an access right
// control.
type darcAction struct {
	key    []byte
	access Access
}

// NewCreate returns a new action to create a DARC.
func NewCreate(access Access) basic.ClientAction {
	return darcAction{access: access}
}

// NewUpdate returns a new action to update a DARC.
func NewUpdate(key []byte, access Access) basic.ClientAction {
	return darcAction{key: key, access: access}
}

// Pack implements encoding.Packable. It returns the protobuf message for the
// action.
func (act darcAction) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	access, err := enc.Pack(act.access)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack access: %v", err)
	}

	pb := &ActionProto{
		Key:    act.key,
		Access: access.(*AccessProto),
	}

	return pb, nil
}

// Fingerprint implements encoding.Fingerprinter. It serializes the DARC action
// into the writer in a deterministic way.
func (act darcAction) Fingerprint(w io.Writer, enc encoding.ProtoMarshaler) error {
	_, err := w.Write(act.key)
	if err != nil {
		return xerrors.Errorf("couldn't write key: %v", err)
	}

	err = act.access.Fingerprint(w, enc)
	if err != nil {
		return xerrors.Errorf("couldn't fingerprint access: %v", err)
	}

	return nil
}

// serverAction is the server-side action for DARCs.
type serverAction struct {
	encoder encoding.ProtoMarshaler
	darcAction
}

// Consume implements basic.ServerAction. It writes the DARC into the page if it
// is allowed to do so, otherwise it returns an error.
func (act serverAction) Consume(ctx basic.Context, page inventory.WritablePage) error {
	accesspb, err := act.encoder.Pack(act.access)
	if err != nil {
		return xerrors.Errorf("couldn't pack access: %v", err)
	}

	key := act.key
	if key == nil {
		// No key defined means a creation request then we use the transaction
		// ID as a unique key for the DARC.
		key = ctx.GetID()
	} else {
		// TODO: access control
	}

	err = page.Write(key, accesspb)
	if err != nil {
		return xerrors.Errorf("couldn't write access: %v", err)
	}

	return nil
}

type actionFactory struct {
	encoder     encoding.ProtoMarshaler
	darcFactory arc.AccessControlFactory
}

// NewActionFactory returns a new instance of the action factory.
func NewActionFactory() basic.ActionFactory {
	return actionFactory{
		encoder:     encoding.NewProtoEncoder(),
		darcFactory: NewFactory(),
	}
}

// FromProto implements basic.ActionFactory. It returns the server action of the
// protobuf message when approriate, otherwise an error.
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
		return nil, xerrors.Errorf("couldn't decode access: %v", err)
	}

	servAccess := serverAction{
		darcAction: darcAction{
			key:    pb.GetKey(),
			access: access.(Access),
		},
		encoder: f.encoder,
	}

	return servAccess, nil
}
