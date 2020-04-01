package byzcoin

import (
	"context"
	"testing"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/crypto"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/ledger/consumer/smartcontract"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minoch"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&BlockPayload{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestLedger_Basic(t *testing.T) {
	manager := minoch.NewManager()

	m, err := minoch.NewMinoch(manager, "A")
	require.NoError(t, err)

	ledger := NewLedger(m, makeConsumer())
	roster := roster{members: []*Ledger{ledger}}

	actor, err := ledger.Listen(roster)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	trs := ledger.Watch(ctx)

	tx, err := smartcontract.NewTransactionFactory([]byte("deadbeef")).
		NewSpawn(simpleContractName, &empty.Empty{})
	require.NoError(t, err)

	err = actor.AddTransaction(tx)
	require.NoError(t, err)

	select {
	case res := <-trs:
		require.NotNil(t, res)
		require.Equal(t, tx.GetID(), res.GetTransactionID())
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout")
	}

	instance, err := ledger.GetInstance(tx.GetID())
	require.NoError(t, err)
	require.Equal(t, tx.GetID(), instance.GetKey())
}

func TestLedger_GetInstance(t *testing.T) {
	ledger := &Ledger{
		bc: fakeBlockchain{},
		proc: &txProcessor{
			inventory: fakeInventory{
				page: &fakePage{},
			},
		},
		consumer: fakeConsumer{},
	}

	instance, err := ledger.GetInstance([]byte{0xab})
	require.NoError(t, err)
	require.NotNil(t, instance)

	ledger.bc = fakeBlockchain{err: xerrors.New("oops")}
	_, err = ledger.GetInstance(nil)
	require.EqualError(t, err, "couldn't read latest block: oops")

	ledger.bc = fakeBlockchain{}
	ledger.proc.inventory = fakeInventory{err: xerrors.New("oops")}
	_, err = ledger.GetInstance(nil)
	require.EqualError(t, err, "couldn't read the page: oops")

	ledger.proc.inventory = fakeInventory{page: &fakePage{err: xerrors.New("oops")}}
	_, err = ledger.GetInstance(nil)
	require.EqualError(t, err, "couldn't read the instance: oops")

	ledger.proc.inventory = fakeInventory{page: &fakePage{}}
	ledger.consumer = fakeConsumer{errFactory: xerrors.New("oops")}
	_, err = ledger.GetInstance(nil)
	require.EqualError(t, err, "couldn't decode instance: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type simpleContract struct{}

const simpleContractName = "simpleContract"

func (c simpleContract) Spawn(ctx smartcontract.SpawnContext) (proto.Message, error) {
	return &empty.Empty{}, nil
}

func (c simpleContract) Invoke(ctx smartcontract.InvokeContext) (proto.Message, error) {
	return &empty.Empty{}, nil
}

func makeConsumer() consumer.Consumer {
	c := smartcontract.NewConsumer()
	c.Register(simpleContractName, simpleContract{})
	return c
}

type addressIterator struct {
	index   int
	members []*Ledger
}

func (i *addressIterator) HasNext() bool {
	return i.index+1 < len(i.members)
}

func (i *addressIterator) GetNext() mino.Address {
	if i.HasNext() {
		i.index++
		return i.members[i.index].addr
	}
	return nil
}

type publicKeyIterator struct {
	index   int
	members []*Ledger
}

func (i *publicKeyIterator) HasNext() bool {
	return i.index+1 < len(i.members)
}

func (i *publicKeyIterator) GetNext() crypto.PublicKey {
	if i.HasNext() {
		i.index++
		return i.members[i.index].signer.GetPublicKey()
	}
	return nil
}

type roster struct {
	members []*Ledger
}

func (r roster) Len() int {
	return len(r.members)
}

func (r roster) Take(...mino.FilterUpdater) mino.Players {
	return roster{members: r.members}
}

func (r roster) AddressIterator() mino.AddressIterator {
	return &addressIterator{index: -1, members: r.members}
}

func (r roster) PublicKeyIterator() crypto.PublicKeyIterator {
	return &publicKeyIterator{index: -1, members: r.members}
}

type fakeBlock struct {
	blockchain.Block
}

func (b fakeBlock) GetIndex() uint64 {
	return 0
}

type fakeBlockchain struct {
	blockchain.Blockchain
	err error
}

func (bc fakeBlockchain) GetBlock() (blockchain.Block, error) {
	return fakeBlock{}, bc.err
}
