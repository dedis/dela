package byzcoin

import (
	"context"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/blockchain/skipchain"
	"go.dedis.ch/dela/consensus/cosipbft"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/cosi/flatcosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/ledger"
	"go.dedis.ch/dela/ledger/byzcoin/memship"
	"go.dedis.ch/dela/ledger/inventory/mem"
	"go.dedis.ch/dela/ledger/transactions"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/gossip"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ -I ../../. --go_out=Mconsensus/viewchange/roster/messages.proto=go.dedis.ch/dela/consensus/viewchange/roster:. ./messages.proto

const (
	initialRoundTime = 50 * time.Millisecond
	timeoutRoundTime = 1 * time.Minute
)

var (
	rosterValueKey = []byte(memship.RosterValueKey)

	rosterLeaderKey = []byte(memship.RosterLeaderKey)
)

// Ledger is a distributed public ledger implemented by using a blockchain. Each
// node is responsible for collecting transactions from clients and propose them
// to the consensus. The blockchain layer will take care of gathering all the
// proposals and create a unified block.
//
// - implements ledger.Ledger
type Ledger struct {
	addr       mino.Address
	signer     crypto.Signer
	bc         blockchain.Blockchain
	gossiper   gossip.Gossiper
	bag        *txBag
	proc       *txProcessor
	viewchange viewchange.ViewChange
	encoder    encoding.ProtoMarshaler
	txFactory  transactions.TransactionFactory
	closing    chan struct{}
	closed     sync.WaitGroup
	initiated  chan error
}

// NewLedger creates a new Byzcoin ledger.
func NewLedger(m mino.Mino, signer crypto.AggregateSigner) *Ledger {
	inventory := mem.NewInventory()
	taskFactory, vc := newtaskFactory(m, signer, inventory)

	txFactory := basic.NewTransactionFactory(signer, taskFactory)

	consensus := cosipbft.NewCoSiPBFT(m, flatcosi.NewFlat(m, signer), vc)

	return &Ledger{
		addr:       m.GetAddress(),
		signer:     signer,
		bc:         skipchain.NewSkipchain(m, consensus),
		gossiper:   gossip.NewFlat(m, rumorFactory{txFactory: txFactory}),
		bag:        newTxBag(),
		proc:       newTxProcessor(txFactory, inventory),
		viewchange: vc,
		encoder:    encoding.NewProtoEncoder(),
		txFactory:  txFactory,
		closing:    make(chan struct{}),
		initiated:  make(chan error, 1),
	}
}

// Listen implements ledger.Ledger. It starts to participate in the blockchain
// and returns an actor that can send transactions.
func (ldgr *Ledger) Listen() (ledger.Actor, error) {
	bcActor, err := ldgr.bc.Listen(ldgr.proc)
	if err != nil {
		return nil, xerrors.Errorf("couldn't start the blockchain: %v", err)
	}

	gossipActor, err := ldgr.gossiper.Listen()
	if err != nil {
		return nil, xerrors.Errorf("couldn't start gossip: %v", err)
	}

	go func() {
		// Wait for the genesis block to be created to start the routines
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		blocks := ldgr.bc.Watch(ctx)

		genesis, err := ldgr.bc.GetBlock()
		if err != nil {
			// Genesis is not stored so it listens for a setup from a
			// participant.
			genesis = <-blocks
			if genesis.GetIndex() != 0 {
				ldgr.initiated <- xerrors.Errorf("expect genesis but got block %d",
					genesis.GetIndex())
				return
			}
		}

		dela.Logger.Trace().
			Hex("hash", genesis.GetHash()).
			Msg("received genesis block")

		authority, err := ldgr.viewchange.GetAuthority()
		if err != nil {
			ldgr.initiated <- xerrors.Errorf("couldn't read chain roster: %v", err)
			return
		}

		// This will start the gossiper with the initial roster.
		gossipActor.SetPlayers(authority)

		ldgr.closed.Add(2)
		close(ldgr.initiated)

		go ldgr.gossipTxs()
		go ldgr.proposeBlocks(bcActor, gossipActor, authority)
	}()

	return newActor(ldgr, bcActor, gossipActor), nil
}

func (ldgr *Ledger) gossipTxs() {
	defer ldgr.closed.Done()

	for {
		select {
		case <-ldgr.closing:
			return
		case rumor := <-ldgr.gossiper.Rumors():
			tx, ok := rumor.(transactions.ClientTransaction)
			if ok {
				ldgr.bag.Add(tx)
			}
		}
	}
}

func (ldgr *Ledger) proposeBlocks(actor blockchain.Actor, ga gossip.Actor, players mino.Players) {
	defer ldgr.closed.Done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blocks := ldgr.bc.Watch(ctx)

	roundTimeout := time.After(initialRoundTime)

	for {
		select {
		case <-ldgr.closing:
			// The actor has been closed.
			return
		case <-roundTimeout:
			// This timeout has two purposes. The very first use will determine
			// the round time before the first block is proposed after a boot.
			// Then it will be used to insure that blocks are still proposed in
			// case of catastrophic failure in the consensus layer (i.e. too
			// many players offline for a while).
			err := ldgr.proposeBlock(actor, players)
			if err != nil {
				dela.Logger.Err(err).Msg("couldn't propose new block")
			}

			roundTimeout = time.After(timeoutRoundTime)
		case block := <-blocks:
			payload, ok := block.GetPayload().(*BlockPayload)
			if !ok {
				dela.Logger.Warn().Msgf("found invalid payload type '%T' != '%T'",
					block.GetPayload(), payload)
				break
			}

			txRes := make([]TransactionResult, len(payload.GetTransactions()))
			for i, txProto := range payload.GetTransactions() {
				tx, err := ldgr.txFactory.FromProto(txProto)
				if err != nil {
					dela.Logger.Warn().Err(err).Msg("couldn't decode transaction")
					return
				}

				txRes[i] = TransactionResult{
					txID:     tx.GetID(),
					Accepted: true,
				}
			}

			ldgr.bag.Remove(txRes...)

			// Update the gossip list of participants with the roster block.
			roster, err := ldgr.viewchange.GetAuthority()
			if err != nil {
				dela.Logger.Err(err).Msg("couldn't read block roster")
			} else {
				ga.SetPlayers(roster)
			}

			// This is executed in a different go routine so that the gathering
			// of transactions can keep on while the block is created.
			// TODO: this should be delayed to allow several blocks to be notified.
			err = ldgr.proposeBlock(actor, players)
			if err != nil {
				dela.Logger.Err(err).Msg("couldn't propose new block")
			}

			roundTimeout = time.After(timeoutRoundTime)
		}
	}
}

func (ldgr *Ledger) proposeBlock(actor blockchain.Actor, players mino.Players) error {
	payload, err := ldgr.stagePayload(ldgr.bag.GetAll())
	if err != nil {
		return xerrors.Errorf("couldn't make the payload: %v", err)
	}

	// Each instance proposes a payload based on the received
	// transactions but it depends on the blockchain implementation
	// if it will be accepted.
	err = actor.Store(payload, players)
	if err != nil {
		return xerrors.Errorf("couldn't send the payload: %v", err)
	}

	return nil
}

// stagePayload creates a payload with the list of transactions by staging a new
// snapshot to the inventory.
func (ldgr *Ledger) stagePayload(txs []transactions.ClientTransaction) (*BlockPayload, error) {
	dela.Logger.Trace().
		Str("addr", ldgr.addr.String()).
		Msgf("staging payload with %d transactions", len(txs))

	payload := &BlockPayload{
		Transactions: make([]*any.Any, len(txs)),
	}

	for i, tx := range txs {
		txpb, err := ldgr.encoder.PackAny(tx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't pack tx: %v", err)
		}

		payload.Transactions[i] = txpb
	}

	page, err := ldgr.proc.process(ldgr.addr, payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't process the txs: %v", err)
	}

	payload.Fingerprint = page.GetFingerprint()

	return payload, nil
}

// Watch implements ledger.Ledger. It listens for new transactions and returns
// the transaction result that can be used to verify if the transaction has been
// accepted or rejected.
func (ldgr *Ledger) Watch(ctx context.Context) <-chan ledger.TransactionResult {
	blocks := ldgr.bc.Watch(ctx)
	results := make(chan ledger.TransactionResult)

	go func() {
		for {
			block, ok := <-blocks
			if !ok {
				dela.Logger.Trace().Msg("watcher is closing")
				return
			}

			payload, ok := block.GetPayload().(*BlockPayload)
			if ok {
				for _, txProto := range payload.GetTransactions() {
					tx, err := ldgr.txFactory.FromProto(txProto)
					if err != nil {
						dela.Logger.Warn().Err(err).Msg("couldn't decode transaction")
						return
					}

					results <- TransactionResult{
						txID:     tx.GetID(),
						Accepted: true,
					}
				}
			}
		}
	}()

	return results
}

type actorLedger struct {
	*Ledger
	bcActor     blockchain.Actor
	gossipActor gossip.Actor
}

func newActor(l *Ledger, a blockchain.Actor, ga gossip.Actor) actorLedger {
	return actorLedger{
		Ledger:      l,
		bcActor:     a,
		gossipActor: ga,
	}
}

func (a actorLedger) HasStarted() <-chan error {
	return a.initiated
}

func (a actorLedger) Setup(players mino.Players) error {
	authority, ok := players.(crypto.CollectiveAuthority)
	if !ok {
		return xerrors.Errorf("players must implement 'crypto.CollectiveAuthority'")
	}

	rosterpb, err := a.encoder.Pack(roster.New(authority))
	if err != nil {
		return xerrors.Errorf("couldn't pack roster: %v", err)
	}

	payload := &GenesisPayload{Roster: rosterpb.(*roster.Roster)}

	page, err := a.proc.setup(payload)
	if err != nil {
		return xerrors.Errorf("couldn't store genesis payload: %v", err)
	}

	payload.Fingerprint = page.GetFingerprint()

	err = a.bcActor.InitChain(payload, authority)
	if err != nil {
		return xerrors.Errorf("couldn't initialize the chain: %v", err)
	}

	return nil
}

// AddTransaction implements ledger.Actor. It sends the transaction towards the
// consensus layer.
func (a actorLedger) AddTransaction(tx transactions.ClientTransaction) error {
	// The gossiper will propagate the transaction to other players but also to
	// the transaction buffer of this player.
	err := a.gossipActor.Add(tx)
	if err != nil {
		return xerrors.Errorf("couldn't propagate the tx: %v", err)
	}

	return nil
}

func (a actorLedger) Close() error {
	close(a.closing)

	err := a.gossipActor.Close()
	if err != nil {
		return xerrors.Errorf("couldn't stop gossiper: %v", err)
	}

	return nil
}

type rumorFactory struct {
	txFactory transactions.TransactionFactory
}

func (f rumorFactory) FromProto(msg proto.Message) (gossip.Rumor, error) {
	return f.txFactory.FromProto(msg)
}
