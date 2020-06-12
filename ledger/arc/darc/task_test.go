package darc

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
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

func TestClientTask_VisitJSON(t *testing.T) {
	task := clientTask{
		key:    []byte{0x1},
		access: NewAccess(),
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(task)
	require.NoError(t, err)
	require.Equal(t, `{"Key":"AQ==","Access":{"Rules":{}}}`, string(data))

	_, err = task.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize access: fake error")
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
	task.key = nil
	err = task.Consume(fakeContext{}, fakePage{call: call})
	require.NoError(t, err)
	require.Equal(t, 2, call.Len())
	require.Equal(t, []byte{0x34}, call.Get(1, 0))

	err = task.Consume(fakeContext{}, fakePage{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't write access: oops")

	task.key = []byte{0x01}
	err = task.Consume(fakeContext{}, fakePage{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't read value: oops")

	task.access.rules[UpdateAccessRule].matches["cat"] = struct{}{}
	err = task.Consume(fakeContext{identity: []byte("cat")}, fakePage{})
	require.EqualError(t, err,
		"no access: couldn't match 'darc_update': couldn't match identity 'cat'")
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

func TestTaskFactory_VisitJSON(t *testing.T) {
	factory := NewTaskFactory()

	ser := json.NewSerializer()

	var task serverTask
	err := ser.Deserialize([]byte(`{"Key":"AQ==","Access":{}}`), factory, &task)
	require.NoError(t, err)
	require.Equal(t, clientTask{key: []byte{0x1}, access: NewAccess()}, task.clientTask)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize task: fake error")

	_, err = factory.VisitJSON(fake.FactoryInput{Serde: fake.NewBadSerializer()})
	require.EqualError(t, err, "couldn't deserialize access: fake error")
}

func TestRegister(t *testing.T) {
	factory := basic.NewTransactionFactory(fake.NewSigner())
	Register(factory, NewTaskFactory())
}

// -----------------------------------------------------------------------------
// Utility functions

var testAccess = Access{
	rules: map[string]expression{
		UpdateAccessRule: {matches: map[string]struct{}{"doggy": {}}},
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

func (page fakePage) Read(key []byte) (serde.Message, error) {
	return testAccess, page.err
}

func (page fakePage) Write(key []byte, value serde.Message) error {
	page.call.Add(key, value)

	return page.err
}

type badArcFactory struct {
	arc.AccessControlFactory
}

func (f badArcFactory) FromProto(proto.Message) (arc.AccessControl, error) {
	return nil, xerrors.New("oops")
}
