package roster

import (
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

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

// -----------------------------------------------------------------------------
// Utility functions

type fakePage struct {
	inventory.Page
	value proto.Message
	err   error
}

func (p fakePage) Read([]byte) (proto.Message, error) {
	return p.value, p.err
}

type fakeInventory struct {
	inventory.Inventory
	value   proto.Message
	err     error
	errPage error
}

func (i fakeInventory) GetPage(uint64) (inventory.Page, error) {
	return fakePage{value: i.value, err: i.errPage}, i.err
}
