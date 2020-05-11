package byzcoin

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/crypto/bls"
	"go.dedis.ch/fabric/encoding"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/ledger"
	"go.dedis.ch/fabric/ledger/arc/darc"
	"go.dedis.ch/fabric/ledger/byzcoin/roster"
	"go.dedis.ch/fabric/ledger/transactions"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minoch"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&BlockPayload{},
		&GenesisPayload{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

// This test checks the basic behaviour of a Byzcoin ledger. The module should
// do the following steps without errors:
// 1. Run n nodes and start to listen for requests
// 2. Setup the ledger on the leader (as we use a leader-based view change)
// 3. Send transactions and accept them.
func TestLedger_Basic(t *testing.T) {
	ledgers, actors, ca := makeLedger(t, 20)
	defer func() {
		for _, actor := range actors {
			require.NoError(t, actor.Close())
		}
	}()

	ro := ca.Take(mino.RangeFilter(0, 19))

	require.NoError(t, actors[0].Setup(ro))

	for _, actor := range actors[:19] {
		err := <-actor.HasStarted()
		require.NoError(t, err)
	}

	signer := bls.NewSigner()
	txFactory := basic.NewTransactionFactory(signer, nil)

	// Send a few transactions..
	for i := 0; i < 2; i++ {
		tx, err := txFactory.New(darc.NewCreate(makeDarc(t, signer)))
		require.NoError(t, err)

		sendTx(t, ledgers[1], actors[5], tx)
	}

	addAddr := ledgers[19].(*Ledger).addr
	addPk := ledgers[19].(*Ledger).signer.GetPublicKey()

	// Execute a roster change tx by removing one of the participants.
	tx, err := txFactory.New(roster.NewAdd(addAddr, addPk))
	require.NoError(t, err)

	sendTx(t, ledgers[1], actors[1], tx)

	err = <-actors[19].HasStarted()
	require.NoError(t, err)

	// Send a few transactions..
	for i := 0; i < 2; i++ {
		tx, err := txFactory.New(darc.NewCreate(makeDarc(t, signer)))
		require.NoError(t, err)

		// 20th participant should now be setup.
		sendTx(t, ledgers[19], actors[10], tx)
	}

	latest, err := ledgers[0].(*Ledger).bc.GetVerifiableBlock()
	require.NoError(t, err)

	latestpb, err := latest.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)

	_, err = ledgers[0].(*Ledger).bc.GetBlockFactory().FromVerifiable(latestpb)
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

func makeLedger(t *testing.T, n int) ([]ledger.Ledger, []ledger.Actor, crypto.CollectiveAuthority) {
	manager := minoch.NewManager()

	minos := make([]mino.Mino, n)
	for i := 0; i < n; i++ {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)

		minos[i] = m
	}

	ca := fake.NewAuthorityFromMino(bls.NewSigner, minos...)
	ledgers := make([]ledger.Ledger, n)
	actors := make([]ledger.Actor, n)
	for i, m := range minos {
		ledger := NewLedger(m, ca.GetSigner(i))
		ledgers[i] = ledger

		actor, err := ledger.Listen()
		require.NoError(t, err)

		actors[i] = actor
	}

	return ledgers, actors, ca
}

func makeDarc(t *testing.T, signer crypto.Signer) darc.Access {
	access := darc.NewAccess()
	access, err := access.Evolve(darc.UpdateAccessRule, signer.GetPublicKey())
	require.NoError(t, err)

	return access
}

func sendTx(t *testing.T, ledger ledger.Ledger, actor ledger.Actor, tx transactions.ClientTransaction) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	txs := ledger.Watch(ctx)

	err := actor.AddTransaction(tx)
	require.NoError(t, err)

	for {
		select {
		case res := <-txs:
			require.NotNil(t, res)
			if bytes.Equal(tx.GetID(), res.GetTransactionID()) {
				return
			}
		case <-time.After(1 * time.Second):
			t.Fatal("timeout when waiting for the transaction")
		}
	}
}
