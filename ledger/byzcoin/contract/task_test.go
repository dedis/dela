package contract

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

func TestSpawnTask_Pack(t *testing.T) {
	task := SpawnTask{
		ContractID: "deadbeef",
		Argument:   &empty.Empty{},
	}

	pb, err := task.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*SpawnTaskProto)(nil), pb)

	taskpb := pb.(*SpawnTaskProto)
	require.Equal(t, task.ContractID, taskpb.GetContractID())
	require.True(t, ptypes.Is(taskpb.GetArgument(), task.Argument))

	_, err = task.Pack(fake.BadMarshalAnyEncoder{})
	require.EqualError(t, err, "couldn't pack argument: fake error")
}

func TestSpawnTask_Fingerprint(t *testing.T) {
	task := SpawnTask{
		ContractID: "deadbeef",
		Argument:   &empty.Empty{},
	}

	buffer := new(bytes.Buffer)
	encoder := encoding.NewProtoEncoder()

	err := task.Fingerprint(buffer, encoder)
	require.NoError(t, err)
	require.Equal(t, "deadbeef{}", buffer.String())

	err = task.Fingerprint(fake.NewBadHash(), encoder)
	require.EqualError(t, err, "couldn't write contract: fake error")

	err = task.Fingerprint(buffer, fake.BadMarshalStableEncoder{})
	require.EqualError(t, err, "couldn't write argument: fake error")
}

func TestInvokeTask_Pack(t *testing.T) {
	task := InvokeTask{
		Key:      []byte{0x01},
		Argument: &empty.Empty{},
	}

	pb, err := task.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*InvokeTaskProto)(nil), pb)

	taskpb := pb.(*InvokeTaskProto)
	require.Equal(t, task.Key, taskpb.GetKey())
	require.True(t, ptypes.Is(taskpb.GetArgument(), task.Argument))

	_, err = task.Pack(fake.BadMarshalAnyEncoder{})
	require.EqualError(t, err, "couldn't pack argument: fake error")
}

func TestInvokeTask_WriteTo(t *testing.T) {
	task := InvokeTask{
		Key:      []byte{0x01},
		Argument: &empty.Empty{},
	}

	buffer := new(bytes.Buffer)
	encoder := encoding.NewProtoEncoder()

	err := task.Fingerprint(buffer, encoder)
	require.NoError(t, err)
	require.Equal(t, "\x01{}", buffer.String())

	err = task.Fingerprint(fake.NewBadHash(), encoder)
	require.EqualError(t, err, "couldn't write key: fake error")

	err = task.Fingerprint(buffer, fake.BadMarshalStableEncoder{})
	require.EqualError(t, err, "couldn't write argument: fake error")
}

func TestDeleteTask_Pack(t *testing.T) {
	task := DeleteTask{
		Key: []byte{0x01},
	}

	pb, err := task.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*DeleteTaskProto)(nil), pb)

	taskpb := pb.(*DeleteTaskProto)
	require.Equal(t, task.Key, taskpb.GetKey())
}

func TestDeleteTask_WriteTo(t *testing.T) {
	task := DeleteTask{
		Key: []byte{0x01},
	}

	buffer := new(bytes.Buffer)
	encoder := encoding.NewProtoEncoder()

	err := task.Fingerprint(buffer, encoder)
	require.NoError(t, err)
	require.Equal(t, "\x01", buffer.String())

	err = task.Fingerprint(fake.NewBadHash(), encoder)
	require.EqualError(t, err, "couldn't write key: fake error")
}

func TestServerTask_Consume(t *testing.T) {
	factory := &fakeAccessFactory{access: &fakeAccess{match: true}}
	contracts := map[string]Contract{
		"fake": fakeContract{},
		"bad":  fakeContract{err: xerrors.New("oops")},
	}

	task := serverTask{
		ClientTask: SpawnTask{ContractID: "fake"},
		contracts:  contracts,
		arcFactory: factory,
		encoder:    encoding.NewProtoEncoder(),
	}

	page := fakePage{
		store: map[string]proto.Message{
			"a":   makeInstance(t),
			"y":   &Instance{ContractID: "bad", AccessControl: []byte("arc")},
			"z":   &Instance{ContractID: "unknown", AccessControl: []byte("arc")},
			"arc": &empty.Empty{},
		},
	}

	// 1. Consume a spawn task.
	err := task.Consume(fakeContext{id: []byte("b")}, page)
	require.NoError(t, err)

	err = task.Consume(fakeContext{id: []byte("a")}, page)
	require.EqualError(t, err, "instance already exists")

	task.ClientTask = SpawnTask{ContractID: "unknown"}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err, "contract 'unknown' not found")

	task.ClientTask = SpawnTask{ContractID: "bad"}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't execute spawn: oops")

	task.ClientTask = SpawnTask{ContractID: "fake"}
	factory.err = xerrors.New("oops")
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"no access: couldn't read access: couldn't decode access: oops")

	factory.err = nil
	task.encoder = fake.BadMarshalAnyEncoder{}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't pack value: fake error")

	// 2. Consume an invoke task.
	task.encoder = encoding.NewProtoEncoder()
	task.ClientTask = InvokeTask{Key: []byte("b")}

	factory.access.calls = make([][]interface{}, 0)
	err = task.Consume(fakeContext{}, page)
	require.NoError(t, err)
	require.Len(t, factory.access.calls, 1)
	require.Equal(t, []arc.Identity{fake.PublicKey{}}, factory.access.calls[0][0])
	require.Equal(t, arc.Compile("fake", "invoke"), factory.access.calls[0][1])

	task.ClientTask = InvokeTask{Key: []byte("c")}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"couldn't read the instance: couldn't read the value: not found")

	task.ClientTask = InvokeTask{Key: []byte("z")}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err, "contract 'unknown' not found")

	task.ClientTask = InvokeTask{Key: []byte("b")}
	factory.err = xerrors.New("oops")
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"no access: couldn't read access: couldn't decode access: oops")

	factory.err = nil
	factory.access.match = false
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"no access: fake.PublicKey is refused to 'fake:invoke' by fakeAccessControl: not authorized")

	factory.access.match = true
	task.ClientTask = InvokeTask{Key: []byte("y")}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't invoke: oops")

	task.ClientTask = InvokeTask{Key: []byte("a")}
	task.encoder = fake.BadMarshalAnyEncoder{}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't pack value: fake error")

	// 3. Consume a delete task.
	task.ClientTask = DeleteTask{Key: []byte("a")}

	err = task.Consume(fakeContext{}, page)
	require.NoError(t, err)

	task.ClientTask = DeleteTask{Key: []byte("c")}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err,
		"couldn't read the instance: couldn't read the value: not found")

	// 4. Consume an invalid task.
	page.err = xerrors.New("oops")
	task.ClientTask = DeleteTask{Key: []byte("a")}
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err, "couldn't write instance to page: oops")

	task.ClientTask = nil
	err = task.Consume(fakeContext{}, page)
	require.EqualError(t, err, "invalid task type '<nil>'")
}

func TestTaskFactory_Register(t *testing.T) {
	factory := NewTaskFactory()

	factory.Register("a", fakeContract{})
	factory.Register("b", fakeContract{})
	require.Len(t, factory.contracts, 2)

	factory.Register("a", fakeContract{})
	require.Len(t, factory.contracts, 2)
}

func TestTaskFactory_FromProto(t *testing.T) {
	factory := NewTaskFactory()

	spawnpb := &SpawnTaskProto{ContractID: "A"}
	task, err := factory.FromProto(spawnpb)
	require.NoError(t, err)
	require.NotNil(t, task)

	spawnAny, err := ptypes.MarshalAny(spawnpb)
	require.NoError(t, err)
	task, err = factory.FromProto(spawnAny)
	require.NoError(t, err)
	require.NotNil(t, task)

	invokepb := &InvokeTaskProto{Key: []byte{0x01}}
	task, err = factory.FromProto(invokepb)
	require.NoError(t, err)
	require.NotNil(t, task)

	deletepb := &DeleteTaskProto{Key: []byte{0x01}}
	task, err = factory.FromProto(deletepb)
	require.NoError(t, err)
	require.NotNil(t, task)

	_, err = factory.FromProto(nil)
	require.EqualError(t, err, "invalid message type '<nil>'")

	factory.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = factory.FromProto(spawnAny)
	require.EqualError(t, err, "couldn't unmarshal message: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeInstance(t *testing.T) *Instance {
	value, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)

	return &Instance{
		ContractID:    "fake",
		AccessControl: []byte("arc"),
		Deleted:       false,
		Value:         value,
	}
}

type fakeContract struct {
	Contract
	err error
}

func (c fakeContract) Spawn(ctx SpawnContext) (proto.Message, []byte, error) {
	ctx.Read([]byte{0xab})
	return &empty.Empty{}, []byte("arc"), c.err
}

func (c fakeContract) Invoke(ctx InvokeContext) (proto.Message, error) {
	ctx.Read([]byte{0xab})
	return &empty.Empty{}, c.err
}

type fakePage struct {
	inventory.WritablePage
	store map[string]proto.Message
	err   error
}

func (page fakePage) Read(key []byte) (proto.Message, error) {
	instance := page.store[string(key)]
	if instance == nil {
		return nil, xerrors.New("not found")
	}

	return instance, nil
}

func (page fakePage) Write(key []byte, value proto.Message) error {
	page.store[string(key)] = value
	return page.err
}

type fakeContext struct {
	id []byte
}

func (ctx fakeContext) GetID() []byte {
	return ctx.id
}

func (ctx fakeContext) GetIdentity() arc.Identity {
	return fake.PublicKey{}
}

type fakeAccess struct {
	arc.AccessControl
	match bool
	calls [][]interface{}
}

func (ac *fakeAccess) Match(rule string, idents ...arc.Identity) error {
	ac.calls = append(ac.calls, []interface{}{idents, rule})
	if ac.match {
		return nil
	}
	return xerrors.New("not authorized")
}

func (ac *fakeAccess) String() string {
	return "fakeAccessControl"
}

type fakeAccessFactory struct {
	arc.AccessControlFactory
	access *fakeAccess
	err    error
}

func (f *fakeAccessFactory) FromProto(proto.Message) (arc.AccessControl, error) {
	return f.access, f.err
}
