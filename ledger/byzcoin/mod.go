package byzcoin

import (
	"context"
	"time"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/blockchain/skipchain"
	"go.dedis.ch/fabric/cosi/flatcosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/crypto/bls"
	"go.dedis.ch/fabric/ledger"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/gossip"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

const (
	initialRoundTime = 50 * time.Millisecond
	timeoutRoundTime = 1 * time.Minute
)

// Ledger is a distributed public ledger implemented by using a blockchain. Each
// node is responsible for collecting transactions from clients and propose them
// to the consensus. The blockchain layer will take care of gathering all the
// proposals and create a unified block.
//
// - implements ledger.Ledger
type Ledger struct {
	addr      mino.Address
	signer    crypto.Signer
	bc        blockchain.Blockchain
	txFactory transactionFactory
	gossiper  gossip.Gossiper
	queue     *txBag
	inventory *txProcessor
}

// NewLedger creates a new Byzcoin ledger.
func NewLedger(mino mino.Mino) *Ledger {
	signer := bls.NewSigner()
	cosi := flatcosi.NewFlat(mino, signer)
	factory := newTransactionFactory()
	decoder := func(pb proto.Message) (gossip.Rumor, error) {
		return factory.FromProto(pb)
	}

	return &Ledger{
		addr:      mino.GetAddress(),
		signer:    signer,
		bc:        skipchain.NewSkipchain(mino, cosi),
		txFactory: factory,
		gossiper:  gossip.NewFlat(mino, decoder),
		queue:     newTxBag(),
		inventory: newValidator(),
	}
}

// GetTransactionFactory implements ledger.Ledger. It ..
func (ldgr *Ledger) GetTransactionFactory() ledger.TransactionFactory {
	return ldgr.txFactory
}

// Listen implements ledger.Ledger. It starts to participate in the blockchain
// and returns an actor that can send transactions.
func (ldgr *Ledger) Listen(players mino.Players) (ledger.Actor, error) {
	bcActor, err := ldgr.bc.Listen(ldgr.inventory)
	if err != nil {
		return nil, xerrors.Errorf("couldn't start the blockchain: %v", err)
	}

	err = bcActor.InitChain(&BlockPayload{}, players)
	if err != nil {
		return nil, xerrors.Errorf("couldn't initialize the chain: %v", err)
	}

	err = ldgr.gossiper.Start(players)
	if err != nil {
		return nil, xerrors.Errorf("couldn't start the gossiper: %v", err)
	}

	go ldgr.routine(bcActor, players)

	return newActor(ldgr.gossiper), err
}

func (ldgr *Ledger) routine(actor blockchain.Actor, players mino.Players) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blocks := ldgr.bc.Watch(ctx)

	roundTimeout := time.After(initialRoundTime)

	for {
		select {
		// TODO: closing
		case rumor := <-ldgr.gossiper.Rumors():
			tx, ok := rumor.(Transaction)
			if ok {
				ldgr.queue.Add(tx)
			}
		case <-roundTimeout:
			// This timeout has two purposes. The very first use will determine
			// the round time before the first block is proposed after a boot.
			// Then it will be used to insure that blocks are still proposed in
			// case of catastrophic failure in the consensus layer (i.e. too
			// many players offline for a while).
			go ldgr.proposeBlock(actor, players)

			roundTimeout = time.After(timeoutRoundTime)
		case block := <-blocks:
			payload, ok := block.GetPayload().(*BlockPayload)
			if !ok {
				fabric.Logger.Warn().Msgf("found invalid payload type '%T' != '%T'",
					block.GetPayload(), payload)
				break
			}

			txRes := make([]TransactionResult, len(payload.GetTxs()))
			for i, pb := range payload.GetTxs() {
				tx, err := ldgr.txFactory.FromProto(pb)
				if err != nil {
					return
				}

				txRes[i] = TransactionResult{
					txID: tx.(Transaction).hash,
				}
			}

			ldgr.queue.Remove(txRes...)

			// This is executed in a different go routine so that the gathering
			// of transactions can keep on while the block is created.
			go ldgr.proposeBlock(actor, players)

			roundTimeout = time.After(timeoutRoundTime)
		}
	}
}

func (ldgr *Ledger) proposeBlock(actor blockchain.Actor, players mino.Players) {
	payload, err := ldgr.stagePayload(ldgr.queue.GetAll())
	if err != nil {
		fabric.Logger.Err(err).Msg("couldn't make the payload")
	}

	// Each instance proposes a payload based on the received
	// transactions but it depends on the blockchain implementation
	// if it will be accepted.
	err = actor.Store(payload, players)
	if err != nil {
		fabric.Logger.Err(err).Msg("couldn't send the payload")
	}
}

// stagePayload creates a payload with the list of transactions by staging a new
// snapshot to the inventory.
func (ldgr *Ledger) stagePayload(txs []Transaction) (*BlockPayload, error) {
	payload := &BlockPayload{
		Txs: make([]*TransactionProto, len(txs)),
	}

	for i, tx := range txs {
		packed, err := tx.Pack()
		if err != nil {
			return nil, xerrors.Errorf("failed to pack tx: %v", err)
		}

		payload.Txs[i] = packed.(*TransactionProto)
	}

	snap, err := ldgr.inventory.process(payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't process the txs: %v", err)
	}

	payload.Root = snap.GetRoot()

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
				fabric.Logger.Debug().Msg("watcher is closing")
				return
			}

			payload, ok := block.GetPayload().(*BlockPayload)
			if ok {
				for _, txProto := range payload.GetTxs() {
					tx, err := ldgr.txFactory.FromProto(txProto)
					if err != nil {
						return
					}

					results <- TransactionResult{
						txID: tx.(Transaction).hash,
					}
				}
			}
		}
	}()

	return results
}

type actor struct {
	gossiper gossip.Gossiper
}

func newActor(g gossip.Gossiper) actor {
	return actor{
		gossiper: g,
	}
}

// AddTransaction implements ledger.Actor. It sends the transaction towards the
// consensus layer.
func (a actor) AddTransaction(in ledger.Transaction) error {
	tx, ok := in.(Transaction)
	if !ok {
		return xerrors.Errorf("message type '%T' but expected '%T'", in, tx)
	}

	// The gossiper will propagate the transaction to other players but also to
	// the transaction buffer of this player.
	err := a.gossiper.Add(tx)
	if err != nil {
		return xerrors.Errorf("couldn't propagate the tx: %v", err)
	}

	return nil
}
