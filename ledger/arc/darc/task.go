package darc

import (
	"io"

	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

const (
	// UpdateAccessRule is the rule to be defined in the DARC to update it.
	UpdateAccessRule = "darc_update"
)

var taskFormats = registry.NewSimpleRegistry()

// RegisterTaskFormat register the engine for the provided format.
func RegisterTaskFormat(c serde.Format, f serde.FormatEngine) {
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
func NewCreate(access Access) ClientTask {
	return ClientTask{access: access}
}

// NewUpdate returns a new task to update a DARC.
func NewUpdate(key []byte, access Access) ClientTask {
	return ClientTask{key: key, access: access}
}

// GetKey returns the key of the access control.
func (t ClientTask) GetKey() []byte {
	return append([]byte{}, t.key...)
}

// GetAccess returns the access control of the task.
func (t ClientTask) GetAccess() Access {
	return t.access
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data of the task.
func (t ClientTask) Serialize(ctx serde.Context) ([]byte, error) {
	format := taskFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, t)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode task: %v", err)
	}

	return data, nil
}

// Fingerprint implements encoding.Fingerprinter. It serializes the client task
// into the writer in a deterministic way.
func (t ClientTask) Fingerprint(w io.Writer) error {
	_, err := w.Write(t.key)
	if err != nil {
		return xerrors.Errorf("couldn't write key: %v", err)
	}

	err = t.access.Fingerprint(w)
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

// NewServerTask creates a new server task with the key and the acces control
// provided.
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
func (t ServerTask) Consume(ctx basic.Context, page inventory.WritablePage) error {
	err := t.access.Match(UpdateAccessRule, ctx.GetIdentity())
	if err != nil {
		// This prevents to update the arc so that no one is allowed to update
		// it in the future.
		return xerrors.New("transaction identity should be allowed to update")
	}

	key := t.key
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

	err = page.Write(key, t.access)
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
func NewTaskFactory() serde.Factory {
	return taskFactory{}
}

// Deserialize implements serde.Factory. It looks up the format and returns the
// message deserialized from the data.
func (f taskFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := taskFormats.Get(ctx.GetFormat())

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode task: %v", err)
	}

	return msg, nil
}

// Register registers the task messages to the transaction factory.
func Register(r basic.TransactionFactory, f serde.Factory) {
	r.Register(ClientTask{}, f)
	r.Register(ServerTask{}, f)
}
