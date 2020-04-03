package byzcoin

import (
	"context"
	"time"

	"github.com/golang/protobuf/proto"
	any "github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/blockchain/skipchain"
	"go.dedis.ch/fabric/cosi/flatcosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/crypto/bls"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger"
	"go.dedis.ch/fabric/ledger/consumer"
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
	addr     mino.Address
	signer   crypto.Signer
	bc       blockchain.Blockchain
	gossiper gossip.Gossiper
	bag      *txBag
	proc     *txProcessor
	consumer consumer.Consumer
	encoder  encoding.ProtoMarshaler
}

// NewLedger creates a new Byzcoin ledger.
func NewLedger(mino mino.Mino, consumer consumer.Consumer) *Ledger {
	signer := bls.NewSigner()
	cosi := flatcosi.NewFlat(mino, signer)
	decoder := func(pb proto.Message) (gossip.Rumor, error) {
		return consumer.GetTransactionFactory().FromProto(pb)
	}

	return &Ledger{
		addr:     mino.GetAddress(),
		signer:   signer,
		bc:       skipchain.NewSkipchain(mino, cosi),
		gossiper: gossip.NewFlat(mino, decoder),
		bag:      newTxBag(),
		proc:     newTxProcessor(consumer),
		consumer: consumer,
		encoder:  encoding.NewProtoEncoder(),
	}
}

// GetInstance implements ledger.Ledger. It returns the instance with the given
// key as of the latest block.
func (ldgr *Ledger) GetInstance(key []byte) (consumer.Instance, error) {
	latest, err := ldgr.bc.GetBlock()
	if err != nil {
		return nil, xerrors.Errorf("couldn't read latest block: %v", err)
	}

	page, err := ldgr.proc.inventory.GetPage(latest.GetIndex())
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the page: %v", err)
	}

	instancepb, err := page.Read(key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the instance: %v", err)
	}

	instance, err := ldgr.consumer.GetInstanceFactory().FromProto(instancepb)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode instance: %v", err)
	}

	return instance, nil
}

// Listen implements ledger.Ledger. It starts to participate in the blockchain
// and returns an actor that can send transactions.
func (ldgr *Ledger) Listen(players mino.Players) (ledger.Actor, error) {
	bcActor, err := ldgr.bc.Listen(ldgr.proc)
	if err != nil {
		return nil, xerrors.Errorf("couldn't start the blockchain: %v", err)
	}

	payload, err := ldgr.stagePayload(nil)
	if err != nil {
		return nil, xerrors.Errorf("couldn't make genesis payload: %v", err)
	}

	err = bcActor.InitChain(payload, players)
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
			tx, ok := rumor.(consumer.Transaction)
			if ok {
				ldgr.bag.Add(tx)
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

			factory := ldgr.consumer.GetTransactionFactory()

			txRes := make([]TransactionResult, len(payload.GetTransactions()))
			for i, txProto := range payload.GetTransactions() {
				tx, err := factory.FromProto(txProto)
				if err != nil {
					fabric.Logger.Warn().Err(err).Msg("couldn't decode transaction")
					return
				}

				txRes[i] = TransactionResult{
					txID:     tx.GetID(),
					Accepted: true,
				}
			}

			ldgr.bag.Remove(txRes...)

			// This is executed in a different go routine so that the gathering
			// of transactions can keep on while the block is created.
			go ldgr.proposeBlock(actor, players)

			roundTimeout = time.After(timeoutRoundTime)
		}
	}
}

func (ldgr *Ledger) proposeBlock(actor blockchain.Actor, players mino.Players) {
	payload, err := ldgr.stagePayload(ldgr.bag.GetAll())
	if err != nil {
		fabric.Logger.Err(err).Msg("couldn't make the payload")
		return
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
func (ldgr *Ledger) stagePayload(txs []consumer.Transaction) (*BlockPayload, error) {
	fabric.Logger.Trace().Msgf("staging payload with %d transactions", len(txs))
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

	page, err := ldgr.proc.process(payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't process the txs: %v", err)
	}

	payload.Footprint = page.GetFootprint()

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
				fabric.Logger.Trace().Msg("watcher is closing")
				return
			}

			payload, ok := block.GetPayload().(*BlockPayload)
			if ok {
				factory := ldgr.consumer.GetTransactionFactory()

				for _, txProto := range payload.GetTransactions() {
					tx, err := factory.FromProto(txProto)
					if err != nil {
						fabric.Logger.Warn().Err(err).Msg("couldn't decode transaction")
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
func (a actor) AddTransaction(tx consumer.Transaction) error {
	// The gossiper will propagate the transaction to other players but also to
	// the transaction buffer of this player.
	err := a.gossiper.Add(tx)
	if err != nil {
		return xerrors.Errorf("couldn't propagate the tx: %v", err)
	}

	return nil
}
