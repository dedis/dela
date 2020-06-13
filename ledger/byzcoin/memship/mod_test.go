package memship

import (
	"bytes"
	"io"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/encoding"
	internal "go.dedis.ch/dela/internal/testing"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&Task{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestTask_VisitJSON(t *testing.T) {
	task := NewTask(fake.NewAuthority(1, fake.NewSigner))

	ser := json.NewSerializer()

	data, err := ser.Serialize(task)
	require.NoError(t, err)
	require.Regexp(t, `{"Authority":\[{"Address":"[^"]+","PublicKey":{}}\]}`, string(data))

	_, err = task.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize authority: fake error")
}

func TestTask_Fingerprint(t *testing.T) {
	task := NewTask(fake.NewAuthority(1, fake.NewSigner)).(clientTask)

	out := new(bytes.Buffer)
	err := task.Fingerprint(out)
	require.NoError(t, err)

	task.authority = badAuthority{}
	err = task.Fingerprint(out)
	require.EqualError(t, err, "couldn't fingerprint authority: oops")
}

func TestTask_Consume(t *testing.T) {
	task := serverTask{
		clientTask: clientTask{
			authority: roster.New(fake.NewAuthority(3, fake.NewSigner)),
		},
		encoder: encoding.NewProtoEncoder(),
	}

	page := fakePage{values: make(map[string]serde.Message)}

	err := task.Consume(nil, page)
	require.NoError(t, err)

	page.errWrite = xerrors.New("oops")
	err = task.Consume(nil, page)
	require.EqualError(t, err, "couldn't write roster: oops")
}

func TestTaskManager_GetChangeSetFactory(t *testing.T) {
	manager := TaskManager{csFactory: fake.MessageFactory{}}
	require.NotNil(t, manager.GetChangeSetFactory())
}

func TestTaskManager_GetAuthority(t *testing.T) {
	manager := TaskManager{
		inventory: fakeInventory{
			value: roster.New(fake.NewAuthority(3, fake.NewSigner)),
		},
		rosterFactory: fakeRosterFactory{},
	}

	authority, err := manager.GetAuthority(0)
	require.NoError(t, err)
	require.Equal(t, 3, authority.Len())

	manager.inventory = fakeInventory{err: xerrors.New("oops")}
	_, err = manager.GetAuthority(1)
	require.EqualError(t, err, "couldn't read page: oops")

	manager.inventory = fakeInventory{errPage: xerrors.New("oops")}
	_, err = manager.GetAuthority(1)
	require.EqualError(t, err, "couldn't read entry: oops")
}

func TestTaskManager_Wait(t *testing.T) {
	manager := TaskManager{
		me: fake.NewAddress(0),
		inventory: fakeInventory{
			value: roster.New(fake.NewAuthority(3, fake.NewSigner)),
		},
		rosterFactory: fakeRosterFactory{},
	}

	allowed := manager.Wait()
	require.True(t, allowed)

	manager.me = fake.NewAddress(1)
	allowed = manager.Wait()
	require.False(t, allowed)

	manager.inventory = fakeInventory{err: xerrors.New("oops")}
	allowed = manager.Wait()
	require.False(t, allowed)
}

func TestTaskManager_Verify(t *testing.T) {
	manager := TaskManager{
		inventory: fakeInventory{
			value: roster.New(fake.NewAuthority(3, fake.NewSigner)),
		},
		rosterFactory: fakeRosterFactory{},
	}

	authority, err := manager.Verify(fake.NewAddress(0), 0)
	require.NoError(t, err)
	require.Equal(t, 3, authority.Len())

	_, err = manager.Verify(fake.NewAddress(1), 0)
	require.EqualError(t, err, "<fake.Address[1]> is not the leader")

	manager.inventory = fakeInventory{err: xerrors.New("oops")}
	_, err = manager.Verify(fake.NewAddress(0), 0)
	require.EqualError(t, err, "couldn't get authority: couldn't read page: oops")
}

func TestTaskManager_FromProto(t *testing.T) {
	factory := NewTaskManager(fakeInventory{}, fake.Mino{}, fake.NewSigner())

	roster := roster.New(fake.NewAuthority(3, fake.NewSigner))
	rosterpb, err := encoding.NewProtoEncoder().PackAny(roster)
	require.NoError(t, err)

	taskany, err := ptypes.MarshalAny(&Task{Authority: rosterpb})
	require.NoError(t, err)

	task, err := factory.FromProto(&Task{Authority: rosterpb})
	require.NoError(t, err)
	require.NotNil(t, task)

	task, err = factory.FromProto(taskany)
	require.NoError(t, err)
	require.NotNil(t, task)

	_, err = factory.FromProto(&empty.Empty{})
	require.EqualError(t, err, "invalid message type '*empty.Empty'")
}

func TestTaskManager_VisitJSON(t *testing.T) {
	factory := NewTaskManager(fakeInventory{}, fake.Mino{}, fake.NewSigner())

	ser := json.NewSerializer()

	var task serverTask
	err := ser.Deserialize([]byte(`{"Authority":[{}]}`), factory, &task)
	require.NoError(t, err)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize task: fake error")

	_, err = factory.VisitJSON(fake.FactoryInput{Serde: fake.NewBadSerializer()})
	require.EqualError(t, err, "couldn't deserialize roster: fake error")
}

func TestRegister(t *testing.T) {
	factory := basic.NewTransactionFactory(fake.NewSigner())
	Register(factory, NewTaskManager(nil, fake.Mino{}, fake.NewSigner()))
}

// -----------------------------------------------------------------------------
// Utility functions

type fakePage struct {
	inventory.WritablePage
	values   map[string]serde.Message
	errRead  error
	errWrite error
	counter  *fake.Counter
}

func (p fakePage) GetIndex() uint64 {
	return 5
}

func (p fakePage) Read(key []byte) (serde.Message, error) {
	if p.errRead != nil {
		defer p.counter.Decrease()
		if p.counter.Done() {
			return nil, p.errRead
		}
	}

	return p.values[string(key)], nil
}

func (p fakePage) Write(key []byte, value serde.Message) error {
	if p.errWrite != nil {
		defer p.counter.Decrease()
		if p.counter.Done() {
			return p.errWrite
		}
	}

	p.values[string(key)] = value
	return nil
}

func (p fakePage) Defer(fn func([]byte)) {
	fn([]byte{0x12})
}

type fakeInventory struct {
	inventory.Inventory
	value   serde.Message
	err     error
	errPage error
}

func (i fakeInventory) Len() uint64 {
	return 1
}

func (i fakeInventory) GetPage(uint64) (inventory.Page, error) {
	values := map[string]serde.Message{
		RosterValueKey: i.value,
	}
	return fakePage{values: values, errRead: i.errPage}, i.err
}

type fakeRosterFactory struct {
	roster.Factory

	err error
}

func (f fakeRosterFactory) FromProto(proto.Message) (viewchange.Authority, error) {
	return roster.New(fake.NewAuthority(3, fake.NewSigner)), f.err
}

type badAuthority struct {
	viewchange.Authority
}

func (a badAuthority) Fingerprint(io.Writer) error {
	return xerrors.New("oops")
}
