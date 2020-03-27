package byzcoin

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

func TestTxProcessor_Validate(t *testing.T) {
	proc := newTxProcessor()
	proc.inventory = fakeInventory{}

	err := proc.Validate(0, &BlockPayload{})
	require.NoError(t, err)

	err = proc.Validate(0, nil)
	require.EqualError(t, err, "message type '<nil>' but expected '*byzcoin.BlockPayload'")

	proc.inventory = fakeInventory{err: xerrors.New("oops")}
	err = proc.Validate(0, &BlockPayload{})
	require.EqualError(t, err, "couldn't stage the transactions: oops")

	proc.inventory = fakeInventory{index: 1}
	err = proc.Validate(0, &BlockPayload{})
	require.EqualError(t, err, "invalid index 1 != 0")

	proc.inventory = fakeInventory{footprint: []byte{0xab}}
	err = proc.Validate(0, &BlockPayload{Footprint: []byte{0xcd}})
	require.EqualError(t, err, "mismatch payload footprint '0xab' != '0xcd'")
}

func TestTxProcessor_Commit(t *testing.T) {
	proc := newTxProcessor()
	proc.inventory = fakeInventory{}

	err := proc.Commit(&BlockPayload{})
	require.NoError(t, err)

	err = proc.Commit(nil)
	require.EqualError(t, err, "message type '<nil>' but expected '*byzcoin.BlockPayload'")

	proc.inventory = fakeInventory{err: xerrors.New("oops")}
	err = proc.Commit(&BlockPayload{Footprint: []byte{0xab}})
	require.EqualError(t, err, "couldn't commit to page '0xab': oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakePage struct {
	inventory.Page
	index     uint64
	footprint []byte
}

func (p fakePage) GetIndex() uint64 {
	return p.index
}

func (p fakePage) GetFootprint() []byte {
	return p.footprint
}

type fakeInventory struct {
	inventory.Inventory
	index     uint64
	footprint []byte
	err       error
}

func (inv fakeInventory) Stage(func(inventory.WritablePage) error) (inventory.Page, error) {
	return fakePage{index: inv.index, footprint: inv.footprint}, inv.err
}

func (inv fakeInventory) Commit([]byte) error {
	return inv.err
}
