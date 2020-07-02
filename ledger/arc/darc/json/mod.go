package json

import (
	"encoding/json"

	"go.dedis.ch/dela/ledger/arc/darc"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	darc.RegisterAccessFormat(serde.FormatJSON, accessFormat{})
	darc.RegisterTaskFormat(serde.FormatJSON, newTaskFormat())
}

// Access is the JSON message for a distributed access control.
type Access struct {
	Rules map[string][]string
}

// ClientTask is a JSON message for a DARC transaction task.
type ClientTask struct {
	Key    []byte
	Access json.RawMessage
}

// AccessFormat is the engine to encode and decode access messages in JSON
// format.
//
// - implements serde.FormatEngine
type accessFormat struct{}

// Encode implements serde.FormatEngine. It returns the JSON representation of
// the access message if appropriate, otherwise an error.
func (f accessFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	access, ok := msg.(darc.Access)
	if !ok {
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	rules := make(map[string][]string)
	for key, expr := range access.GetRules() {
		rules[key] = expr.GetMatches()
	}

	m := Access{
		Rules: rules,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It returns the message of the JSON data
// if appropritate, otherwise it returns an error.
func (f accessFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := Access{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize access: %v", err)
	}

	opts := make([]darc.AccessOption, 0, len(m.Rules))
	for rule, matches := range m.Rules {
		opts = append(opts, darc.WithRule(rule, matches))
	}

	access := darc.NewAccess(opts...)

	return access, nil
}

// TaskFormat is an engine to encode and decode task messages in JSON format.
//
// - implements serde.FormatEngine
type taskFormat struct {
	accessFormat serde.FormatEngine
}

func newTaskFormat() taskFormat {
	return taskFormat{
		accessFormat: accessFormat{},
	}
}

// Encode implements serde.FormatEngine. It returns the JSON data of the task
// message if appropriate, otherwise an error.
func (f taskFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	var act darc.ClientTask
	switch in := msg.(type) {
	case darc.ClientTask:
		act = in
	case darc.ServerTask:
		act = in.ClientTask
	default:
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	access, err := f.accessFormat.Encode(ctx, act.GetAccess())
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize access: %v", err)
	}

	m := ClientTask{
		Key:    act.GetKey(),
		Access: access,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It returns the task message of the JSON
// data if appropriate, otherwise an error.
func (f taskFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := ClientTask{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize task: %v", err)
	}

	access, err := f.accessFormat.Decode(ctx, m.Access)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize access: %v", err)
	}

	task := darc.NewServerTask(m.Key, access.(darc.Access))

	return task, nil
}
