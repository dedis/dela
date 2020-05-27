package byzcoin

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/encoding"
	internal "go.dedis.ch/dela/internal/testing"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger"
	"go.dedis.ch/dela/ledger/arc/darc"
	"go.dedis.ch/dela/ledger/byzcoin/roster"
	"go.dedis.ch/dela/ledger/transactions"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/gossip"
	"go.dedis.ch/dela/mino/minoch"
	"golang.org/x/xerrors"
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

	// Execute a roster change tx by adding the remaining participant.
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

func TestLedger_Listen(t *testing.T) {
	ledger := &Ledger{
		initiated:  make(chan error, 1),
		closing:    make(chan struct{}),
		bc:         fakeBlockchain{},
		governance: fakeGovernance{},
		gossiper:   fakeGossiper{},
	}

	actor, err := ledger.Listen()
	require.NoError(t, err)
	require.NotNil(t, actor)
	require.NoError(t, actor.Close())
	waitClose(ledger)

	ledger.bc = fakeBlockchain{errListen: xerrors.New("oops")}
	_, err = ledger.Listen()
	require.EqualError(t, err, "couldn't start the blockchain: oops")
	waitClose(ledger)

	blocks := make(chan blockchain.Block, 1)
	blocks <- fakeBlock{index: 1}
	ledger.bc = fakeBlockchain{blocks: blocks, errBlock: xerrors.New("oops")}
	ledger.initiated = make(chan error, 1)
	_, err = ledger.Listen()
	require.NoError(t, err)
	err = waitClose(ledger)
	require.EqualError(t, err, "expect genesis but got block 1")

	ledger.bc = fakeBlockchain{}
	ledger.governance = fakeGovernance{err: xerrors.New("oops")}
	ledger.initiated = make(chan error, 1)
	_, err = ledger.Listen()
	require.NoError(t, err)
	err = waitClose(ledger)
	require.EqualError(t, err, "couldn't read chain roster: oops")

	ledger.governance = fakeGovernance{}
	ledger.gossiper = fakeGossiper{err: xerrors.New("oops")}
	_, err = ledger.Listen()
	require.EqualError(t, err, "couldn't start gossip: oops")
}

func TestLedger_GossipTxs(t *testing.T) {
	rumors := make(chan gossip.Rumor)

	ledger := &Ledger{
		closing:  make(chan struct{}),
		bag:      newTxBag(),
		gossiper: fakeGossiper{rumors: rumors, err: xerrors.New("oops")},
	}

	ledger.closed.Add(1)
	go func() {
		ledger.gossipTxs()
	}()

	rumors <- fakeTx{id: []byte{0x01}}
	rumors <- fakeTx{id: []byte{0x01}}
	require.Len(t, ledger.bag.GetAll(), 1)
	rumors <- fakeTx{id: []byte{0x02}}
	rumors <- fakeTx{id: []byte{0x02}}
	require.Len(t, ledger.bag.GetAll(), 2)

	close(ledger.closing)
	ledger.closed.Wait()
}

func TestActor_Setup(t *testing.T) {
	actor := actorLedger{
		Ledger: &Ledger{
			encoder:    encoding.NewProtoEncoder(),
			proc:       newTxProcessor(nil, fakeInventory{}),
			governance: fakeGovernance{},
		},
		bcActor: fakeActor{},
	}

	err := actor.Setup(fake.NewAuthority(3, fake.NewSigner))
	require.NoError(t, err)

	err = actor.Setup(mino.NewAddresses())
	require.EqualError(t, err, "players must implement 'crypto.CollectiveAuthority'")

	actor.encoder = fake.BadPackEncoder{}
	err = actor.Setup(fake.NewAuthority(3, fake.NewSigner))
	require.EqualError(t, err, "couldn't pack roster: fake error")

	actor.encoder = encoding.NewProtoEncoder()
	actor.proc = newTxProcessor(nil, fakeInventory{err: xerrors.New("oops")})
	err = actor.Setup(fake.NewAuthority(3, fake.NewSigner))
	require.EqualError(t, err,
		"couldn't store genesis payload: couldn't stage page: oops")

	actor.proc = newTxProcessor(nil, fakeInventory{})
	actor.bcActor = fakeActor{err: xerrors.New("oops")}
	err = actor.Setup(fake.NewAuthority(3, fake.NewSigner))
	require.EqualError(t, err, "couldn't initialize the chain: oops")
}

func TestActor_AddTransaction(t *testing.T) {
	actor := &actorLedger{
		gossipActor: fakeGossipActor{},
	}

	err := actor.AddTransaction(fakeTx{})
	require.NoError(t, err)

	actor.gossipActor = fakeGossipActor{err: xerrors.New("oops")}
	err = actor.AddTransaction(fakeTx{})
	require.EqualError(t, err, "couldn't propagate the tx: oops")
}

func TestActor_Close(t *testing.T) {
	actor := &actorLedger{
		Ledger: &Ledger{
			closing: make(chan struct{}),
		},
		gossipActor: fakeGossipActor{},
	}

	err := actor.Close()
	require.NoError(t, err)

	actor.closing = make(chan struct{})
	actor.gossipActor = fakeGossipActor{err: xerrors.New("oops")}
	err = actor.Close()
	require.EqualError(t, err, "couldn't stop gossiper: oops")
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

func waitClose(ledger *Ledger) error {
	err := <-ledger.initiated
	ledger.closed.Wait()
	return err
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
		case <-time.After(5 * time.Second):
			t.Fatal("timeout when waiting for the transaction")
		}
	}
}

type fakeBlock struct {
	blockchain.Block
	index uint64
}

func (b fakeBlock) GetIndex() uint64 {
	return b.index
}

func (b fakeBlock) GetHash() []byte {
	return []byte{0x12}
}

type fakeBlockchain struct {
	blockchain.Blockchain
	blocks    chan blockchain.Block
	errListen error
	errBlock  error
}

func (bc fakeBlockchain) Listen(blockchain.PayloadProcessor) (blockchain.Actor, error) {
	return nil, bc.errListen
}

func (bc fakeBlockchain) GetBlock() (blockchain.Block, error) {
	return fakeBlock{}, bc.errBlock
}

func (bc fakeBlockchain) Watch(context.Context) <-chan blockchain.Block {
	return bc.blocks
}

type fakeGovernance struct {
	viewchange.Governance
	err error
}

func (gov fakeGovernance) GetAuthorityFactory() viewchange.AuthorityFactory {
	return roster.NewRosterFactory(nil, nil)
}

func (gov fakeGovernance) GetAuthority(index uint64) (viewchange.EvolvableAuthority, error) {
	return fake.NewAuthority(3, fake.NewSigner), gov.err
}

type fakeGossipActor struct {
	err error
}

func (a fakeGossipActor) SetPlayers(mino.Players) {}

func (a fakeGossipActor) Add(gossip.Rumor) error {
	return a.err
}

func (a fakeGossipActor) Close() error {
	return a.err
}

type fakeGossiper struct {
	gossip.Gossiper
	rumors chan gossip.Rumor
	err    error
}

func (g fakeGossiper) Listen() (gossip.Actor, error) {
	return fakeGossipActor{}, g.err
}

func (g fakeGossiper) Rumors() <-chan gossip.Rumor {
	return g.rumors
}

type fakeActor struct {
	blockchain.Actor
	err error
}

func (a fakeActor) InitChain(proto.Message, mino.Players) error {
	return a.err
}
