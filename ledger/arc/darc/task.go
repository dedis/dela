package darc

import (
	"io"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"golang.org/x/xerrors"
)

const (
	// UpdateAccessRule is the rule to be defined in the DARC to update it.
	UpdateAccessRule = "darc_update"
)

// clientTask is the client task of a transaction that will allow an authorized
// identity to create or update a DARC.
//
// - implements basic.ClientTask
type clientTask struct {
	key    []byte
	access Access
}

// TODO: client factory

// NewCreate returns a new task to create a DARC.
func NewCreate(access Access) basic.ClientTask {
	return clientTask{access: access}
}

// NewUpdate returns a new task to update a DARC.
func NewUpdate(key []byte, access Access) basic.ClientTask {
	return clientTask{key: key, access: access}
}

// Pack implements encoding.Packable. It returns the protobuf message for the
// task.
func (act clientTask) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	access, err := enc.Pack(act.access)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack access: %v", err)
	}

	pb := &Task{
		Key:    act.key,
		Access: access.(*AccessProto),
	}

	return pb, nil
}

// Fingerprint implements encoding.Fingerprinter. It serializes the client task
// into the writer in a deterministic way.
func (act clientTask) Fingerprint(w io.Writer, enc encoding.ProtoMarshaler) error {
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

// serverTask is the server task for a DARC transaction.
//
// - implements basic.ServerTask
type serverTask struct {
	clientTask
	encoder     encoding.ProtoMarshaler
	darcFactory arc.AccessControlFactory
}

// Consume implements basic.ServerTask. It writes the DARC into the page if it
// is allowed to do so, otherwise it returns an error.
func (act serverTask) Consume(ctx basic.Context, page inventory.WritablePage) error {
	accesspb, err := act.encoder.Pack(act.access)
	if err != nil {
		return xerrors.Errorf("couldn't pack access: %v", err)
	}

	err = act.access.Match(UpdateAccessRule, ctx.GetIdentity())
	if err != nil {
		// This prevents to update the arc so that no one is allowed to update
		// it in the future.
		return xerrors.New("transaction identity should be allowed to update")
	}

	key := act.key
	if key == nil {
		// No key defined means a creation request then we use the transaction
		// ID as a unique key for the DARC.
		key = ctx.GetID()
	} else {
		value, err := page.Read(key)
		if err != nil {
			return xerrors.Errorf("couldn't read value: %v", err)
		}

		access, err := act.darcFactory.FromProto(value)
		if err != nil {
			return xerrors.Errorf("couldn't decode access: %v", err)
		}

		err = access.Match(UpdateAccessRule, ctx.GetIdentity())
		if err != nil {
			return xerrors.Errorf("no access: %v", err)
		}
	}

	err = page.Write(key, accesspb)
	if err != nil {
		return xerrors.Errorf("couldn't write access: %v", err)
	}

	return nil
}

// taskFactory is a factory to instantiate darc server tasks from protobuf
// messages.
//
// - implements basic.TaskFactory
type taskFactory struct {
	encoder     encoding.ProtoMarshaler
	darcFactory arc.AccessControlFactory
}

// NewTaskFactory returns a new instance of the task factory.
func NewTaskFactory() basic.TaskFactory {
	return taskFactory{
		encoder:     encoding.NewProtoEncoder(),
		darcFactory: NewFactory(),
	}
}

// FromProto implements basic.TaskFactory. It returns the server task of the
// protobuf message when approriate, otherwise an error.
func (f taskFactory) FromProto(in proto.Message) (basic.ServerTask, error) {
	var pb *Task
	switch msg := in.(type) {
	case *Task:
		pb = msg
	default:
		return nil, xerrors.Errorf("invalid message type '%T'", in)
	}

	access, err := f.darcFactory.FromProto(pb.GetAccess())
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode access: %v", err)
	}

	servAccess := serverTask{
		encoder:     f.encoder,
		darcFactory: f.darcFactory,
		clientTask: clientTask{
			key:    pb.GetKey(),
			access: access.(Access),
		},
	}

	return servAccess, nil
}
