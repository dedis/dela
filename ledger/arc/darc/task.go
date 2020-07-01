package darc

import (
	"io"

	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"go.dedis.ch/dela/serdeng"
	"go.dedis.ch/dela/serdeng/registry"
	"golang.org/x/xerrors"
)

const (
	// UpdateAccessRule is the rule to be defined in the DARC to update it.
	UpdateAccessRule = "darc_update"
)

var taskFormats = registry.NewSimpleRegistry()

func RegisterTaskFormat(c serdeng.Codec, f serdeng.Format) {
	taskFormats.Register(c, f)
}

// ClientTask is the client task of a transaction that will allow an authorized
// identity to create or update a DARC.
//
// - implements basic.ClientTask
type ClientTask struct {
	key    []byte
	access Access
}

// NewCreate returns a new task to create a DARC.
func NewCreate(access Access) basic.ClientTask {
	return ClientTask{access: access}
}

// NewUpdate returns a new task to update a DARC.
func NewUpdate(key []byte, access Access) basic.ClientTask {
	return ClientTask{key: key, access: access}
}

func (t ClientTask) GetKey() []byte {
	return append([]byte{}, t.key...)
}

func (t ClientTask) GetAccess() Access {
	return t.access
}

// Serialize implements serde.Message.
func (act ClientTask) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := taskFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, act)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Fingerprint implements encoding.Fingerprinter. It serializes the client task
// into the writer in a deterministic way.
func (act ClientTask) Fingerprint(w io.Writer) error {
	_, err := w.Write(act.key)
	if err != nil {
		return xerrors.Errorf("couldn't write key: %v", err)
	}

	err = act.access.Fingerprint(w)
	if err != nil {
		return xerrors.Errorf("couldn't fingerprint access: %v", err)
	}

	return nil
}

// ServerTask is the server task for a DARC transaction.
//
// - implements basic.ServerTask
type ServerTask struct {
	ClientTask
}

func NewServerTask(key []byte, access Access) ServerTask {
	return ServerTask{
		ClientTask: ClientTask{
			key:    key,
			access: access,
		},
	}
}

// Consume implements basic.ServerTask. It writes the DARC into the page if it
// is allowed to do so, otherwise it returns an error.
func (act ServerTask) Consume(ctx basic.Context, page inventory.WritablePage) error {
	err := act.access.Match(UpdateAccessRule, ctx.GetIdentity())
	if err != nil {
		// This prevents to update the arc so that no one is allowed to update
		// it in the future.
		return xerrors.New("transaction identity should be allowed to update")
	}

	key := act.key
	if len(key) == 0 {
		// No key defined means a creation request then we use the transaction
		// ID as a unique key for the DARC.
		key = ctx.GetID()
	} else {
		value, err := page.Read(key)
		if err != nil {
			return xerrors.Errorf("couldn't read value: %v", err)
		}

		access, ok := value.(Access)
		if !ok {
			return xerrors.Errorf("invalid message type '%T'", value)
		}

		err = access.Match(UpdateAccessRule, ctx.GetIdentity())
		if err != nil {
			return xerrors.Errorf("no access: %v", err)
		}
	}

	err = page.Write(key, act.access)
	if err != nil {
		return xerrors.Errorf("couldn't write access: %v", err)
	}

	return nil
}

// taskFactory is a factory to instantiate darc server tasks from protobuf
// messages.
//
// - implements basic.TaskFactory
type taskFactory struct{}

// NewTaskFactory returns a new instance of the task factory.
func NewTaskFactory() serdeng.Factory {
	return taskFactory{}
}

// Deserialize implements serde.Factory.
func (f taskFactory) Deserialize(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	format := taskFormats.Get(ctx.GetName())

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

// Register registers the task messages to the transaction factory.
func Register(r basic.TransactionFactory, f serdeng.Factory) {
	r.Register(ClientTask{}, f)
	r.Register(ServerTask{}, f)
}
