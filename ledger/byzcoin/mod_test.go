package byzcoin

import (
	"context"
	"testing"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/crypto/bls"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/ledger/arc/darc"
	"go.dedis.ch/fabric/ledger/arc/darc/contract"
	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/ledger/consumer/smartcontract"
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

	ca := fake.NewAuthorityFromMino(bls.NewSigner, m)

	ledger := NewLedger(m, ca.GetSigner(0), makeConsumer())

	actor, err := ledger.Listen(ca)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	trs := ledger.Watch(ctx)

	txFactory := smartcontract.NewTransactionFactory(bls.NewSigner())

	// Try to create a DARC with the conode key pair.
	tx, err := txFactory.New(contract.NewGenesisAction())
	require.NoError(t, err)

	err = actor.AddTransaction(tx)
	require.NoError(t, err)

	select {
	case res := <-trs:
		require.NotNil(t, res)
		require.Equal(t, tx.GetID(), res.GetTransactionID())
	case <-time.After(1 * time.Second):
		t.Fatal("timeout 1")
	}

	instance, err := ledger.GetInstance(tx.GetID())
	require.NoError(t, err)
	require.Equal(t, tx.GetID(), instance.GetKey())
	require.Equal(t, tx.GetID(), instance.GetArcID())
	require.IsType(t, (*darc.AccessControlProto)(nil), instance.GetValue())

	// Then update it.
	tx, err = txFactory.New(contract.NewUpdateAction(tx.GetID()))
	require.NoError(t, err)

	err = actor.AddTransaction(tx)
	require.NoError(t, err)

	select {
	case res := <-trs:
		require.NotNil(t, res)
		require.Equal(t, tx.GetID(), res.GetTransactionID())
	case <-time.After(1 * time.Second):
		t.Fatal("timeout 2")
	}
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

func makeConsumer() consumer.Consumer {
	c := smartcontract.NewConsumer()
	contract.RegisterContract(c)

	return c
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
