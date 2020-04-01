package byzcoin

import (
	"testing"

	proto "github.com/golang/protobuf/proto"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
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
		"couldn't stage new page: couldn't decode tx: oops")

	proc.consumer = fakeConsumer{err: xerrors.New("oops")}
	_, err = proc.process(payload)
	require.EqualError(t, err,
		"couldn't stage new page: couldn't consume tx: oops")

	proc.consumer = fakeConsumer{}
	proc.encoder = badPackEncoder{}
	_, err = proc.process(payload)
	require.EqualError(t, err,
		"couldn't stage new page: couldn't pack instance: oops")

	proc.encoder = encoding.NewProtoEncoder()
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

type badPackEncoder struct {
	encoding.ProtoEncoder
}

func (e badPackEncoder) Pack(encoding.Packable) (proto.Message, error) {
	return nil, xerrors.New("oops")
}

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

func (p *fakePage) Read([]byte) (proto.Message, error) {
	return &empty.Empty{}, p.err
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
	consumer.TransactionFactory
	err error
}

func (f fakeTxFactory) FromProto(proto.Message) (consumer.Transaction, error) {
	return nil, f.err
}

type fakeInstance struct {
	consumer.Instance
	key []byte
	err error
}

func (i fakeInstance) GetKey() []byte {
	return i.key
}

func (i fakeInstance) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, i.err
}

type fakeInstanceFactory struct {
	consumer.InstanceFactory
	err error
}

func (f fakeInstanceFactory) FromProto(proto.Message) (consumer.Instance, error) {
	return fakeInstance{}, f.err
}

type fakeConsumer struct {
	consumer.Consumer
	key         []byte
	err         error
	errFactory  error
	errInstance error
}

func (c fakeConsumer) GetTransactionFactory() consumer.TransactionFactory {
	return fakeTxFactory{err: c.errFactory}
}

func (c fakeConsumer) GetInstanceFactory() consumer.InstanceFactory {
	return fakeInstanceFactory{err: c.errFactory}
}

func (c fakeConsumer) Consume(consumer.Transaction,
	inventory.Page) (consumer.Instance, error) {

	return fakeInstance{key: c.key, err: c.errInstance}, c.err
}
