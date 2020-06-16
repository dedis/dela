package darc

import (
	"io"

	"go.dedis.ch/dela/ledger/arc/darc/json"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

const (
	// UpdateAccessRule is the rule to be defined in the DARC to update it.
	UpdateAccessRule = "darc_update"
)

// ClientTask is the client task of a transaction that will allow an authorized
// identity to create or update a DARC.
//
// - implements basic.ClientTask
type clientTask struct {
	serde.UnimplementedMessage

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

// VisitJSON implements serde.Message. It returns the JSON message for the task.
func (act clientTask) VisitJSON(ser serde.Serializer) (interface{}, error) {
	access, err := ser.Serialize(act.access)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize access: %v", err)
	}

	m := json.ClientTask{
		Key:    act.key,
		Access: access,
	}

	return m, nil
}

// Fingerprint implements encoding.Fingerprinter. It serializes the client task
// into the writer in a deterministic way.
func (act clientTask) Fingerprint(w io.Writer) error {
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

// serverTask is the server task for a DARC transaction.
//
// - implements basic.ServerTask
type serverTask struct {
	clientTask
	darcFactory serde.Factory
}

// Consume implements basic.ServerTask. It writes the DARC into the page if it
// is allowed to do so, otherwise it returns an error.
func (act serverTask) Consume(ctx basic.Context, page inventory.WritablePage) error {
	err := act.access.Match(UpdateAccessRule, ctx.GetIdentity())
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
type taskFactory struct {
	serde.UnimplementedFactory

	darcFactory serde.Factory
}

// NewTaskFactory returns a new instance of the task factory.
func NewTaskFactory() serde.Factory {
	return taskFactory{
		darcFactory: NewFactory(),
	}
}

// VisitJSON implements serde.Factory. It deserializes the server task.
func (f taskFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.ClientTask{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize task: %v", err)
	}

	var access Access
	err = in.GetSerializer().Deserialize(m.Access, f.darcFactory, &access)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize access: %v", err)
	}

	task := serverTask{
		darcFactory: f.darcFactory,
		clientTask: clientTask{
			key:    m.Key,
			access: access,
		},
	}

	return task, nil
}

// Register registers the task messages to the transaction factory.
func Register(r basic.TransactionFactory, f serde.Factory) {
	r.Register(clientTask{}, f)
	r.Register(serverTask{}, f)
}
