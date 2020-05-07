package roster

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

func TestClientTask_GetChangeSet(t *testing.T) {
	task := NewRemove([]uint32{1, 3, 4}).(clientTask)

	changeset := task.GetChangeSet()
	require.Equal(t, []uint32{1, 3, 4}, changeset.Remove)
}

func TestClientTask_Pack(t *testing.T) {
	task := NewRemove([]uint32{1})

	taskpb, err := task.Pack(nil)
	require.NoError(t, err)
	require.Equal(t, []uint32{1}, taskpb.(*Task).GetRemove())
}

func TestClientTask_Fingerprint(t *testing.T) {
	task := NewRemove([]uint32{0x02, 0x01, 0x03})

	buffer := new(bytes.Buffer)

	err := task.Fingerprint(buffer, nil)
	require.NoError(t, err)
	require.Equal(t, "\x02\x00\x00\x00\x01\x00\x00\x00\x03\x00\x00\x00", buffer.String())

	err = task.Fingerprint(fake.NewBadHash(), nil)
	require.EqualError(t, err, "couldn't write remove indices: fake error")
}

func TestServerTask_Consume(t *testing.T) {
	task := serverTask{
		clientTask:    clientTask{remove: []uint32{2}},
		rosterFactory: NewRosterFactory(fake.AddressFactory{}, fake.PublicKeyFactory{}),
		encoder:       encoding.NewProtoEncoder(),
	}

	roster := task.rosterFactory.New(fake.NewAuthority(3, fake.NewSigner))
	rosterpb, err := roster.Pack(task.encoder)
	require.NoError(t, err)

	values := map[string]proto.Message{
		RosterValueKey: rosterpb,
		// Change set is not set yet.
	}

	err = task.Consume(nil, fakePage{values: values})
	require.NoError(t, err)

	changesetpb := values[RosterChangeSetKey]
	require.NotNil(t, changesetpb)
	require.Equal(t, uint64(5), changesetpb.(*ChangeSet).GetIndex())
	require.Equal(t, []uint32{2}, changesetpb.(*ChangeSet).GetRemove())

	task.clientTask.remove = []uint32{4, 2, 4, 2, 3, 2, 4, 6, 4, 2}
	err = task.Consume(nil, fakePage{values: values})
	require.NoError(t, err)

	changesetpb = values[RosterChangeSetKey]
	require.Equal(t, []uint32{6, 4, 3, 2}, changesetpb.(*ChangeSet).GetRemove())

	err = task.Consume(nil, fakePage{errRead: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't read roster: oops")

	err = task.Consume(nil, fakePage{values: map[string]proto.Message{}})
	require.EqualError(t, err, "couldn't decode roster: invalid message type '<nil>'")

	task.encoder = fake.BadPackEncoder{}
	err = task.Consume(nil, fakePage{values: values})
	require.EqualError(t, err, "couldn't encode roster: fake error")

	task.encoder = encoding.NewProtoEncoder()
	err = task.Consume(nil, fakePage{values: values, errWrite: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't write roster: oops")

	page := fakePage{
		values:  values,
		errRead: xerrors.New("oops"),
		counter: &fake.Counter{Value: 1},
	}

	err = task.Consume(nil, page)
	require.EqualError(t, err, "couldn't update change set: couldn't read from page: oops")

	page.errRead = nil
	page.errWrite = xerrors.New("oops")
	page.counter.Value = 1
	err = task.Consume(nil, page)
	require.EqualError(t, err, "couldn't update change set: couldn't write to page: oops")
}

func TestTaskManager_GetAuthorityFactory(t *testing.T) {
	factory := NewRosterFactory(nil, nil)
	manager := NewTaskManager(factory, nil)

	require.NotNil(t, manager.GetAuthorityFactory())
}

func TestTaskManager_GetAuthority(t *testing.T) {
	factory := NewRosterFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})

	roster := factory.New(fake.NewAuthority(3, fake.NewSigner))
	rosterpb, err := roster.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)

	manager := NewTaskManager(factory, fakeInventory{value: rosterpb})

	authority, err := manager.GetAuthority(3)
	require.NoError(t, err)
	require.Equal(t, 3, authority.Len())

	manager.inventory = fakeInventory{err: xerrors.New("oops")}
	_, err = manager.GetAuthority(3)
	require.EqualError(t, err, "couldn't read page: oops")

	manager.inventory = fakeInventory{errPage: xerrors.New("oops")}
	_, err = manager.GetAuthority(3)
	require.EqualError(t, err, "couldn't read roster: oops")

	manager.inventory = fakeInventory{}
	_, err = manager.GetAuthority(3)
	require.EqualError(t, err, "couldn't decode roster: invalid message type '<nil>'")
}

func TestTaskManager_GetChangeSet(t *testing.T) {
	manager := NewTaskManager(nil, fakeInventory{value: &ChangeSet{
		Index:  5,
		Remove: []uint32{3, 4},
	}})

	changeset, err := manager.GetChangeSet(5)
	require.NoError(t, err)
	require.Len(t, changeset.Remove, 2)

	changeset, err = manager.GetChangeSet(6)
	require.NoError(t, err)
	require.Len(t, changeset.Remove, 0)

	manager.inventory = fakeInventory{err: xerrors.New("oops")}
	_, err = manager.GetChangeSet(0)
	require.EqualError(t, err, "couldn't read page: oops")

	manager.inventory = fakeInventory{errPage: xerrors.New("oops")}
	_, err = manager.GetChangeSet(0)
	require.EqualError(t, err, "couldn't read from page: oops")
}

func TestTaskManager_FromProto(t *testing.T) {
	manager := NewTaskManager(nil, nil)

	task, err := manager.FromProto(&Task{})
	require.NoError(t, err)
	require.NotNil(t, task)

	taskAny, err := ptypes.MarshalAny(&Task{})
	require.NoError(t, err)

	task, err = manager.FromProto(taskAny)
	require.NoError(t, err)
	require.NotNil(t, task)

	_, err = manager.FromProto(&empty.Empty{})
	require.EqualError(t, err, "invalid message type '*empty.Empty'")

	manager.encoder = fake.BadUnmarshalAnyEncoder{}
	_, err = manager.FromProto(taskAny)
	require.EqualError(t, err, "couldn't unmarshal message: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakePage struct {
	inventory.WritablePage
	values   map[string]proto.Message
	errRead  error
	errWrite error
	counter  *fake.Counter
}

func (p fakePage) GetIndex() uint64 {
	return 5
}

func (p fakePage) Read(key []byte) (proto.Message, error) {
	if p.errRead != nil {
		defer p.counter.Decrease()
		if p.counter.Done() {
			return nil, p.errRead
		}
	}

	return p.values[string(key)], nil
}

func (p fakePage) Write(key []byte, value proto.Message) error {
	if p.errWrite != nil {
		defer p.counter.Decrease()
		if p.counter.Done() {
			return p.errWrite
		}
	}

	p.values[string(key)] = value
	return nil
}

type fakeInventory struct {
	inventory.Inventory
	value   proto.Message
	err     error
	errPage error
}

func (i fakeInventory) GetPage(uint64) (inventory.Page, error) {
	values := map[string]proto.Message{
		RosterValueKey:     i.value,
		RosterChangeSetKey: i.value,
	}
	return fakePage{values: values, errRead: i.errPage}, i.err
}
