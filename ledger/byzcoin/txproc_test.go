package byzcoin

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/ledger/transactions"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestTxProcessor_Validate(t *testing.T) {
	proc := newTxProcessor(MessageFactory{}, fakeInventory{})

	_, err := proc.InvokeValidate(Blueprint{})
	require.NoError(t, err)

	_, err = proc.InvokeValidate(nil)
	require.EqualError(t, err, "invalid message type '<nil>'")

	blueprint := Blueprint{transactions: []transactions.ServerTransaction{
		fakeTx{err: xerrors.New("oops")},
	}}
	_, err = proc.InvokeValidate(blueprint)
	require.EqualError(t, err,
		"couldn't stage the transactions: couldn't stage new page: couldn't consume tx: oops")
}

func TestTxProcessor_Process(t *testing.T) {
	proc := newTxProcessor(MessageFactory{}, fakeInventory{page: &fakePage{index: 999}})

	page, err := proc.process(BlockPayload{})
	require.NoError(t, err)
	require.Equal(t, uint64(999), page.GetIndex())

	payload := BlockPayload{transactions: []transactions.ServerTransaction{}}

	proc.inventory = fakeInventory{page: &fakePage{}}
	page, err = proc.process(payload)
	require.NoError(t, err)
	require.NotNil(t, page)
}

func TestTxProcessor_Commit(t *testing.T) {
	proc := newTxProcessor(MessageFactory{}, fakeInventory{})

	err := proc.InvokeCommit(BlockPayload{})
	require.NoError(t, err)

	err = proc.InvokeCommit(nil)
	require.EqualError(t, err, "invalid message type '<nil>'")

	proc.inventory = fakeInventory{
		errCommit: xerrors.New("oops"),
		page:      &fakePage{fingerprint: []byte{0xab}},
	}
	err = proc.InvokeCommit(BlockPayload{root: []byte{0xab}})
	require.EqualError(t, err, "couldn't commit to page '0xab': oops")

	proc.inventory = fakeInventory{errPage: xerrors.New("oops")}
	err = proc.InvokeCommit(GenesisPayload{})
	require.EqualError(t, err,
		"couldn't stage genesis: couldn't stage page: couldn't write roster: oops")

	proc.inventory = fakeInventory{index: 1}
	err = proc.InvokeCommit(GenesisPayload{})
	require.EqualError(t, err, "index 0 expected but got 1")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakePage struct {
	inventory.WritablePage
	index       uint64
	fingerprint []byte
	err         error
	value       serde.Message
	calls       [][]interface{}
}

func (p *fakePage) GetIndex() uint64 {
	return p.index
}

func (p *fakePage) GetFingerprint() []byte {
	return p.fingerprint
}

func (p *fakePage) Read([]byte) (serde.Message, error) {
	return p.value, p.err
}

func (p *fakePage) Write(key []byte, value serde.Message) error {
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
	errCommit   error
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
	return inv.errCommit
}
