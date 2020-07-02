package json

import (
	"encoding/json"

	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/ledger/byzcoin/memship"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	memship.RegisterTaskFormat(serde.FormatJSON, taskFormat{})
}

// Task is the JSON message for the client task.
type Task struct {
	Authority json.RawMessage
}

// TaskFormat is the engine to encode and decode tasks in JSON format.
//
// - implements serde.FormatEngine
type taskFormat struct{}

// Encode implements serde.FormatEngine. It returns the JSON representation of
// the task.
func (f taskFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	var task memship.ClientTask
	switch in := msg.(type) {
	case memship.ClientTask:
		task = in
	case memship.ServerTask:
		task = in.ClientTask
	default:
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	authority, err := task.GetAuthority().Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize authority: %v", err)
	}

	m := Task{
		Authority: authority,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the task from the data in
// JSON format if appropriate, otherwise returns an error.
func (f taskFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := Task{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize task: %v", err)
	}

	factory := ctx.GetFactory(memship.RosterKey{})

	fac, ok := factory.(viewchange.AuthorityFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory of type '%T'", factory)
	}

	roster, err := fac.AuthorityOf(ctx, m.Authority)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize roster: %v", err)
	}

	task := memship.NewServerTask(roster)

	return task, nil
}
