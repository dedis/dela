package byzcoin

import (
	"testing"

	proto "github.com/golang/protobuf/proto"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

func TestTxProcessor_Validate(t *testing.T) {
	proc := newTxProcessor(nil, fakeInventory{})

	err := proc.Validate(0, &BlockPayload{})
	require.NoError(t, err)

	err = proc.Validate(0, &GenesisPayload{})
	require.NoError(t, err)

	err = proc.Validate(0, nil)
	require.EqualError(t, err, "invalid message type '<nil>'")

	proc.inventory = fakeInventory{err: xerrors.New("oops")}
	err = proc.Validate(0, &BlockPayload{})
	require.EqualError(t, err,
		"couldn't stage the transactions: couldn't stage new page: oops")

	proc.inventory = fakeInventory{errPage: xerrors.New("oops")}
	err = proc.Validate(0, &GenesisPayload{})
	require.EqualError(t, err,
		"couldn't stage genesis: couldn't stage page: couldn't write roster: oops")

	proc.inventory = fakeInventory{index: 1}
	err = proc.Validate(0, &BlockPayload{})
	require.EqualError(t, err, "invalid index 1 != 0")

	err = proc.Validate(0, &GenesisPayload{})
	require.EqualError(t, err, "index 0 expected but got 1")

	proc.inventory = fakeInventory{fingerprint: []byte{0xab}}
	err = proc.Validate(0, &BlockPayload{Fingerprint: []byte{0xcd}})
	require.EqualError(t, err, "mismatch payload fingerprint '0xab' != '0xcd'")
}

func TestTxProcessor_Process(t *testing.T) {
	proc := newTxProcessor(nil, fakeInventory{page: &fakePage{index: 999}})

	page, err := proc.process(&BlockPayload{})
	require.NoError(t, err)
	require.Equal(t, uint64(999), page.GetIndex())

	payload := &BlockPayload{Transactions: []*any.Any{{}}}

	proc.inventory = fakeInventory{page: &fakePage{}}
	page, err = proc.process(payload)
	require.NoError(t, err)
	require.NotNil(t, page)
}

func TestTxProcessor_Commit(t *testing.T) {
	proc := newTxProcessor(nil, fakeInventory{})

	err := proc.Commit(&BlockPayload{})
	require.NoError(t, err)

	err = proc.Commit(nil)
	require.EqualError(t, err, "invalid message type '<nil>'")

	proc.inventory = fakeInventory{err: xerrors.New("oops")}
	err = proc.Commit(&BlockPayload{Fingerprint: []byte{0xab}})
	require.EqualError(t, err, "couldn't commit to page '0xab': oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakePage struct {
	inventory.WritablePage
	index       uint64
	fingerprint []byte
	err         error
	value       proto.Message
	calls       [][]interface{}
}

func (p *fakePage) GetIndex() uint64 {
	return p.index
}

func (p *fakePage) GetFingerprint() []byte {
	return p.fingerprint
}

func (p *fakePage) Read([]byte) (proto.Message, error) {
	return p.value, p.err
}

func (p *fakePage) Write(key []byte, value proto.Message) error {
	p.calls = append(p.calls, []interface{}{key, value})
	return p.err
}

type fakeInventory struct {
	inventory.Inventory
	index       uint64
	fingerprint []byte
	page        *fakePage
	err         error
	errPage     error
}

func (inv fakeInventory) GetPage(index uint64) (inventory.Page, error) {
	if inv.page != nil {
		return inv.page, inv.err
	}
	return nil, inv.err
}

func (inv fakeInventory) GetStagingPage([]byte) inventory.Page {
	if inv.page != nil {
		return inv.page
	}
	return nil
}

func (inv fakeInventory) Stage(f func(inventory.WritablePage) error) (inventory.Page, error) {
	p := &fakePage{
		index:       inv.index,
		fingerprint: inv.fingerprint,
		err:         inv.errPage,
	}

	err := f(p)
	if err != nil {
		return nil, err
	}

	return p, inv.err
}

func (inv fakeInventory) Commit([]byte) error {
	return inv.err
}
