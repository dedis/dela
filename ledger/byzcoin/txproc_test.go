package byzcoin

import (
	"testing"

	proto "github.com/golang/protobuf/proto"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/ledger"
	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

func TestTxProcessor_Validate(t *testing.T) {
	proc := newTxProcessor(fakeConsumer{})
	proc.inventory = fakeInventory{}

	err := proc.Validate(0, &BlockPayload{})
	require.NoError(t, err)

	err = proc.Validate(0, nil)
	require.EqualError(t, err,
		"message type '<nil>' but expected '*byzcoin.BlockPayload'")

	proc.inventory = fakeInventory{err: xerrors.New("oops")}
	err = proc.Validate(0, &BlockPayload{})
	require.EqualError(t, err,
		"couldn't stage the transactions: couldn't stage new page: oops")

	proc.inventory = fakeInventory{index: 1}
	err = proc.Validate(0, &BlockPayload{})
	require.EqualError(t, err, "invalid index 1 != 0")

	proc.inventory = fakeInventory{footprint: []byte{0xab}}
	err = proc.Validate(0, &BlockPayload{Footprint: []byte{0xcd}})
	require.EqualError(t, err, "mismatch payload footprint '0xab' != '0xcd'")
}

func TestTxProcessor_Process(t *testing.T) {
	proc := newTxProcessor(nil)
	proc.inventory = fakeInventory{page: &fakePage{index: 999}}
	proc.consumer = fakeConsumer{key: []byte{0xab}}

	page, err := proc.process(&BlockPayload{})
	require.NoError(t, err)
	require.Equal(t, uint64(999), page.GetIndex())

	payload := &BlockPayload{Transactions: []*any.Any{{}}}

	proc.inventory = fakeInventory{}
	page, err = proc.process(payload)
	require.NoError(t, err)
	require.Len(t, page.(*fakePage).calls, 1)
	require.Equal(t, []byte{0xab}, page.(*fakePage).calls[0][0])

	proc.consumer = fakeConsumer{errFactory: xerrors.New("oops")}
	_, err = proc.process(payload)
	require.EqualError(t, err,
		"couldn't stage new page: couldn't decode transaction: oops")

	proc.consumer = fakeConsumer{err: xerrors.New("oops")}
	_, err = proc.process(payload)
	require.EqualError(t, err,
		"couldn't stage new page: couldn't consume tx: oops")

	proc.consumer = fakeConsumer{}
	proc.inventory = fakeInventory{errPage: xerrors.New("oops")}
	_, err = proc.process(payload)
	require.EqualError(t, err,
		"couldn't stage new page: couldn't write instances: oops")
}

func TestTxProcessor_Commit(t *testing.T) {
	proc := newTxProcessor(nil)
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
	inventory.WritablePage
	index     uint64
	footprint []byte
	err       error
	calls     [][]interface{}
}

func (p *fakePage) GetIndex() uint64 {
	return p.index
}

func (p *fakePage) GetFootprint() []byte {
	return p.footprint
}

func (p *fakePage) Write(key []byte, value proto.Message) error {
	p.calls = append(p.calls, []interface{}{key, value})
	return p.err
}

type fakeInventory struct {
	inventory.Inventory
	index     uint64
	footprint []byte
	page      *fakePage
	err       error
	errPage   error
}

func (inv fakeInventory) GetStagingPage([]byte) inventory.Page {
	if inv.page != nil {
		return inv.page
	}
	return nil
}

func (inv fakeInventory) Stage(f func(inventory.WritablePage) error) (inventory.Page, error) {
	p := &fakePage{
		index:     inv.index,
		footprint: inv.footprint,
		err:       inv.errPage,
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

type fakeTxFactory struct {
	ledger.TransactionFactory
	err error
}

func (f fakeTxFactory) FromProto(proto.Message) (ledger.Transaction, error) {
	return nil, f.err
}

type fakeConsumer struct {
	consumer.Consumer
	key        []byte
	err        error
	errFactory error
}

func (c fakeConsumer) GetTransactionFactory() ledger.TransactionFactory {
	return fakeTxFactory{err: c.errFactory}
}

func (c fakeConsumer) Consume(ledger.Transaction, inventory.Page) (consumer.Output, error) {
	return consumer.Output{Key: c.key}, c.err
}
