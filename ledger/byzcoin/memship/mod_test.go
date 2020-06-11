package memship

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	internal "go.dedis.ch/dela/internal/testing"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/ledger/transactions/basic"
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

func TestTaskManager_GetAuthority(t *testing.T) {
	manager := TaskManager{
		inventory:     fakeInventory{},
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

	manager.inventory = fakeInventory{}
	manager.rosterFactory = fakeRosterFactory{err: xerrors.New("oops")}
	_, err = manager.GetAuthority(1)
	require.EqualError(t, err, "couldn't decode roster: oops")
}

func TestTaskManager_Wait(t *testing.T) {
	manager := TaskManager{
		me:            fake.NewAddress(0),
		inventory:     fakeInventory{},
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
		inventory:     fakeInventory{},
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

func TestRegister(t *testing.T) {
	factory := basic.NewTransactionFactory(fake.NewSigner())
	Register(factory, NewTaskManager(nil, fake.Mino{}, fake.NewSigner()))
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

func (i fakeInventory) Len() uint64 {
	return 1
}

func (i fakeInventory) GetPage(uint64) (inventory.Page, error) {
	values := map[string]proto.Message{
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
