package json

import (
	"encoding/json"

	"go.dedis.ch/dela/ledger/arc/darc"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

func init() {
	darc.RegisterAccessFormat(serdeng.CodecJSON, accessFormat{})
	darc.RegisterTaskFormat(serdeng.CodecJSON, newTaskFormat())
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

type accessFormat struct{}

func (f accessFormat) Encode(ctx serdeng.Context, msg serdeng.Message) ([]byte, error) {
	access, ok := msg.(darc.Access)
	if !ok {
		return nil, xerrors.New("invalid access")
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
		return nil, err
	}

	return data, nil
}

func (f accessFormat) Decode(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
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

type taskFormat struct {
	accessFormat serdeng.Format
}

func newTaskFormat() taskFormat {
	return taskFormat{
		accessFormat: accessFormat{},
	}
}

func (f taskFormat) Encode(ctx serdeng.Context, msg serdeng.Message) ([]byte, error) {
	var act darc.ClientTask
	switch in := msg.(type) {
	case darc.ClientTask:
		act = in
	case darc.ServerTask:
		act = in.ClientTask
	default:
		return nil, xerrors.New("invalid client task")
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
		return nil, err
	}

	return data, nil
}

func (f taskFormat) Decode(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
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
