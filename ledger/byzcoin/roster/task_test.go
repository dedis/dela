package roster

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/consensus/viewchange"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

func TestClientTask_GetChangeSet(t *testing.T) {
	task := NewRemove([]uint32{1, 3, 1, 3, 4}).(clientTask)

	changeset := task.GetChangeSet()
	require.Equal(t, []uint32{4, 3, 1}, changeset.Remove)

	task = NewAdd(fake.NewAddress(0), fake.PublicKey{}).(clientTask)

	changeset = task.GetChangeSet()
	require.Len(t, changeset.Add, 1)
}

func TestClientTask_Pack(t *testing.T) {
	task := NewRemove([]uint32{1})

	taskpb, err := task.Pack(nil)
	require.NoError(t, err)
	require.Equal(t, []uint32{1}, taskpb.(*Task).GetRemove())

	task = NewAdd(fake.NewAddress(0), fake.PublicKey{})
	taskpb, err = task.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.NotNil(t, taskpb.(*Task).GetAddr())
	require.NotNil(t, taskpb.(*Task).GetPublicKey())

	task = NewAdd(fake.NewBadAddress(), nil)
	_, err = task.Pack(nil)
	require.EqualError(t, err, "couldn't marshal address: fake error")

	task = NewAdd(fake.NewAddress(0), fake.PublicKey{})
	_, err = task.Pack(fake.BadPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack public key: fake error")
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
		bag:           &changeSetBag{},
	}

	roster := task.rosterFactory.New(fake.NewAuthority(3, fake.NewSigner))
	rosterpb, err := roster.Pack(task.encoder)
	require.NoError(t, err)

	values := map[string]proto.Message{
		RosterValueKey: rosterpb,
	}

	err = task.Consume(nil, fakePage{values: values})
	require.NoError(t, err)
	require.Len(t, task.bag.store, 1)

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
	manager := NewTaskManager(nil, nil)

	changeset, err := manager.GetChangeSet(fakeBlock{payload: fakePayload{}})
	require.NoError(t, err)
	require.Len(t, changeset.Remove, 0)

	manager.bag.index = 1
	manager.bag.store = map[[32]byte]viewchange.ChangeSet{
		{0x12}: {Remove: []uint32{1}},
	}
	changeset, err = manager.GetChangeSet(fakeBlock{payload: fakePayload{}})
	require.NoError(t, err)
	require.Len(t, changeset.Remove, 1)

	_, err = manager.GetChangeSet(fakeProposal{})
	require.EqualError(t, err, "proposal must implement blockchain.Block")

	_, err = manager.GetChangeSet(fakeBlock{payload: &empty.Empty{}})
	require.EqualError(t, err, "payload must implement roster.Payload")
}

func TestTaskManager_FromProto(t *testing.T) {
	factory := NewRosterFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	manager := NewTaskManager(factory, nil)

	task, err := manager.FromProto(&Task{Addr: []byte{0x1}})
	require.NoError(t, err)
	require.NotNil(t, task)

	task, err = manager.FromProto(&Task{Addr: []byte{}, PublicKey: &any.Any{}})
	require.NoError(t, err)
	require.NotNil(t, task.(serverTask).player)

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

	factory = NewRosterFactory(fake.AddressFactory{}, fake.NewBadPublicKeyFactory())
	manager = NewTaskManager(factory, nil)
	_, err = manager.FromProto(&Task{Addr: []byte{}, PublicKey: &any.Any{}})
	require.EqualError(t, err, "couldn't decode public key: fake error")
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

func (p fakePage) Defer(fn func([]byte)) {
	fn([]byte{0x12})
}

type fakeInventory struct {
	inventory.Inventory
	value   proto.Message
	err     error
	errPage error
}

func (i fakeInventory) GetPage(uint64) (inventory.Page, error) {
	values := map[string]proto.Message{
		RosterValueKey: i.value,
	}
	return fakePage{values: values, errRead: i.errPage}, i.err
}

type fakePayload struct {
	proto.Message
}

func (p fakePayload) GetFingerprint() []byte {
	return []byte{0x12}
}

type fakeBlock struct {
	consensus.Proposal
	payload proto.Message
}

func (fakeBlock) GetIndex() uint64 {
	return 1
}

func (b fakeBlock) GetPayload() proto.Message {
	return b.payload
}

type fakeProposal struct {
	consensus.Proposal
}
