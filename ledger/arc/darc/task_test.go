package darc

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/inventory"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"golang.org/x/xerrors"
)

func TestClientTask_Pack(t *testing.T) {
	task := clientTask{
		key:    []byte{0x01},
		access: NewAccess(),
	}

	pb, err := task.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*Task)(nil), pb)

	taskpb := pb.(*Task)
	require.Equal(t, task.key, taskpb.GetKey())

	_, err = task.Pack(fake.BadPackEncoder{})
	require.EqualError(t, err, "couldn't pack access: fake error")
}

func TestClientTask_Fingerprint(t *testing.T) {
	task := clientTask{
		key: []byte{0x01},
		access: Access{rules: map[string]expression{
			"\x02": {matches: map[string]struct{}{"\x03": {}}},
		}},
	}

	buffer := new(bytes.Buffer)

	err := task.Fingerprint(buffer, encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.Equal(t, "\x01\x02\x03", buffer.String())

	err = task.Fingerprint(fake.NewBadHash(), nil)
	require.EqualError(t, err, "couldn't write key: fake error")

	err = task.Fingerprint(fake.NewBadHashWithDelay(1), nil)
	require.EqualError(t, err,
		"couldn't fingerprint access: couldn't write key: fake error")
}

func TestServerTask_Consume(t *testing.T) {
	access, err := NewAccess().Evolve(UpdateAccessRule, fakeIdentity{buffer: []byte("doggy")})
	require.NoError(t, err)

	task := serverTask{
		encoder:     encoding.NewProtoEncoder(),
		darcFactory: NewFactory(),
		clientTask:  clientTask{key: []byte{0x01}, access: access},
	}

	call := &fake.Call{}
	err = task.Consume(fakeContext{}, fakePage{call: call})
	require.NoError(t, err)
	require.Equal(t, 1, call.Len())
	// Key is provided so it's an update.
	require.Equal(t, []byte{0x01}, call.Get(0, 0))

	// No key thus it's a creation.
	task.clientTask.key = nil
	err = task.Consume(fakeContext{}, fakePage{call: call})
	require.NoError(t, err)
	require.Equal(t, 2, call.Len())
	require.Equal(t, []byte{0x34}, call.Get(1, 0))

	task.encoder = fake.BadPackEncoder{}
	err = task.Consume(fakeContext{}, fakePage{})
	require.EqualError(t, err, "couldn't pack access: fake error")

	task.encoder = encoding.NewProtoEncoder()
	err = task.Consume(fakeContext{}, fakePage{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't write access: oops")

	task.clientTask.key = []byte{0x01}
	err = task.Consume(fakeContext{}, fakePage{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't read value: oops")

	task.darcFactory = badArcFactory{}
	err = task.Consume(fakeContext{}, fakePage{})
	require.EqualError(t, err, "couldn't decode access: oops")

	task.darcFactory = NewFactory()
	task.access.rules[UpdateAccessRule].matches["cat"] = struct{}{}
	err = task.Consume(fakeContext{identity: []byte("cat")}, fakePage{})
	require.EqualError(t, err,
		"no access: couldn't match 'darc:update': couldn't match identity 'cat'")
}

func TestTaskFactory_FromProto(t *testing.T) {
	factory := NewTaskFactory().(taskFactory)

	taskpb := &Task{Key: []byte{0x02}, Access: &AccessProto{}}
	task, err := factory.FromProto(taskpb)
	require.NoError(t, err)
	require.IsType(t, serverTask{}, task)

	_, err = factory.FromProto(nil)
	require.EqualError(t, err, "invalid message type '<nil>'")

	factory.darcFactory = badArcFactory{}
	_, err = factory.FromProto(&Task{})
	require.EqualError(t, err, "couldn't decode access: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

var testAccess = &AccessProto{
	Rules: map[string]*Expression{
		UpdateAccessRule: {Matches: []string{"doggy"}},
	},
}

type fakeContext struct {
	basic.Context
	identity []byte
}

func (ctx fakeContext) GetID() []byte {
	return []byte{0x34}
}

func (ctx fakeContext) GetIdentity() arc.Identity {
	if ctx.identity != nil {
		return fakeIdentity{buffer: ctx.identity}
	}
	return fakeIdentity{buffer: []byte("doggy")}
}

type fakePage struct {
	inventory.WritablePage
	call *fake.Call
	err  error
}

func (page fakePage) Read(key []byte) (proto.Message, error) {
	return testAccess, page.err
}

func (page fakePage) Write(key []byte, value proto.Message) error {
	page.call.Add(key, value)

	return page.err
}

type badArcFactory struct {
	arc.AccessControlFactory
}

func (f badArcFactory) FromProto(proto.Message) (arc.AccessControl, error) {
	return nil, xerrors.New("oops")
}
