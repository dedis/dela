package byzcoin

import (
	"context"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/blockchain/skipchain"
	"go.dedis.ch/fabric/cosi/flatcosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/crypto/bls"
	"go.dedis.ch/fabric/ledger"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

const (
	roundTime = 50 * time.Millisecond
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
	chTxs     chan Transaction
}

// NewLedger creates a new Byzcoin ledger.
func NewLedger(mino mino.Mino) *Ledger {
	signer := bls.NewSigner()
	cosi := flatcosi.NewFlat(mino, signer)

	return &Ledger{
		addr:      mino.GetAddress(),
		signer:    signer,
		bc:        skipchain.NewSkipchain(mino, cosi),
		txFactory: transactionFactory{},
		chTxs:     make(chan Transaction, 100),
	}
}

// GetTransactionFactory implements ledger.Ledger. It ..
func (l *Ledger) GetTransactionFactory() ledger.TransactionFactory {
	return transactionFactory{}
}

// Listen implements ledger.Ledger. It starts to participate in the blockchain
// and returns an actor that can send transactions.
func (l *Ledger) Listen(players mino.Players) (ledger.Actor, error) {
	actor, err := l.bc.Listen(newValidator())
	if err != nil {
		return nil, xerrors.Errorf("couldn't start the blockchain: %v", err)
	}

	// TODO: init with multiple nodes
	err = actor.InitChain(&empty.Empty{}, players)
	if err != nil {
		return nil, xerrors.Errorf("couldn't initialize the chain: %v", err)
	}

	go func() {
		buffer := make(map[Digest]Transaction)
		round := time.After(roundTime)

		for {
			select {
			case tx := <-l.chTxs:
				buffer[tx.hash] = tx
			case <-round:
				// TODO: update buffer based on stored transactions.
				txs := buffer
				buffer = make(map[Digest]Transaction)

				payload, err := l.makePayload(txs)
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
		}
	}()

	return newActor(l.chTxs), err
}

func (l *Ledger) makePayload(txs map[Digest]Transaction) (*BlockPayload, error) {
	payload := &BlockPayload{
		Txs: make([]*TransactionProto, 0, len(txs)),
	}

	for _, tx := range txs {
		packed, err := tx.Pack()
		if err != nil {
			return nil, err
		}

		payload.Txs = append(payload.Txs, packed.(*TransactionProto))
	}

	return payload, nil
}

// Watch implements ledger.Ledger. It listens for new transactions and returns
// the transaction result that can be used to verify if the transaction has been
// accepted or rejected.
func (l *Ledger) Watch(ctx context.Context) <-chan ledger.TransactionResult {
	blocks := l.bc.Watch(ctx)
	results := make(chan ledger.TransactionResult)

	go func() {
		for {
			block, ok := <-blocks
			if !ok {
				fabric.Logger.Debug().Msg("watcher is closing")
				return
			}

			payload := block.GetPayload().(*BlockPayload)

			for _, pb := range payload.GetTxs() {
				tx, err := l.txFactory.FromProto(pb)
				if err != nil {
					return
				}

				results <- TransactionResult{
					txID: tx.(Transaction).hash,
				}
			}
		}
	}()

	return results
}

type actor struct {
	ch chan Transaction
}

func newActor(ch chan Transaction) actor {
	return actor{
		ch: ch,
	}
}

// AddTransaction implements ledger.Actor. It sends the transaction towards the
// consensus layer. The context can be used to pass a timeout or to cancel the
// request.
func (a actor) AddTransaction(ctx context.Context, in ledger.Transaction) error {
	tx, ok := in.(Transaction)
	if !ok {
		return xerrors.Errorf("invalid message type '%T'", in)
	}

	// TODO: txs gossiping
	a.ch <- tx

	return nil
}

type validator struct{}

func newValidator() validator {
	return validator{}
}

func (v validator) Validate(data proto.Message) error {
	// TODO: implements
	// TODO: return more than just an error to produce tx results
	return nil
}

func (v validator) Commit(data proto.Message) error {
	// TODO: implements
	return nil
}
