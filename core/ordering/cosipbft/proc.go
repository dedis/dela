package cosipbft

import (
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/pbft"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/tap/pool"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

var (
	keyRoster = [32]byte{}
)

// Processor processes the messages to run a collective signing PBFT consensus.
type processor struct {
	mino.UnsupportedHandler
	types.MessageFactory

	pbftsm      pbft.StateMachine
	tree        hashtree.Tree
	pool        pool.Pool
	watcher     blockchain.Observable
	rosterFac   viewchange.AuthorityFactory
	hashFactory crypto.HashFactory

	context serde.Context
	genesis blockstore.GenesisStore
	blocks  blockstore.BlockStore

	started chan struct{}
}

func newProcessor() *processor {
	return &processor{
		watcher: blockchain.NewWatcher(),
		context: json.NewContext(),
		started: make(chan struct{}),
	}
}

// Invoke implements cosi.Reactor. It processes the messages from the collective
// signature module. The messages are either from the the prepare or the commit
// phase.
func (h *processor) Invoke(from mino.Address, msg serde.Message) ([]byte, error) {
	switch in := msg.(type) {
	case types.BlockMessage:
		if h.pbftsm.GetState() == pbft.InitialState {
			roster, err := h.getCurrentRoster()
			if err != nil {
				return nil, xerrors.Errorf("read roster failed: %v", err)
			}

			err = h.pbftsm.PrePrepare(roster)
			if err != nil {
				return nil, xerrors.Errorf("pbft pre-prepare failed: %v", err)
			}
		}

		digest, err := h.pbftsm.Prepare(in.GetBlock(), h.tree)
		if err != nil {
			return nil, xerrors.Errorf("pbft prepare failed: %v", err)
		}

		return digest[:], nil
	case types.CommitMessage:
		err := h.pbftsm.Commit(in.GetID(), in.GetSignature())
		if err != nil {
			return nil, xerrors.Errorf("pbft commit failed: %v", err)
		}

		buffer, err := in.GetSignature().MarshalBinary()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal signature: %v", err)
		}

		return buffer, nil
	default:
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}
}

// Process implements mino.Handler. It processes the messages from the RPC.
func (h *processor) Process(req mino.Request) (serde.Message, error) {
	switch msg := req.Message.(type) {
	case types.GenesisMessage:
		roster, err := msg.GetGenesis().GetRoster().Serialize(h.context)
		if err != nil {
			return nil, xerrors.Errorf("encode roster failed: %v", err)
		}

		stageTree, err := h.tree.Stage(func(snap store.Snapshot) error {
			snap.Set(keyRoster[:], roster)
			return nil
		})
		if err != nil {
			return nil, xerrors.Errorf("tree stage failed: %v", err)
		}

		tree, err := stageTree.Commit()
		if err != nil {
			return nil, xerrors.Errorf("tree commit failed: %v", err)
		}

		h.tree = tree
		err = h.genesis.Set(*msg.GetGenesis())
		if err != nil {
			return nil, xerrors.Errorf("set genesis failed: %v", err)
		}

		h.watcher.Notify(ordering.Event{Index: 0})

		close(h.started)
	case types.DoneMessage:
		tree, err := h.pbftsm.Finalize(msg.GetID(), msg.GetSignature())
		if err != nil {
			return nil, xerrors.Errorf("pbftsm finalized failed: %v", err)
		}

		h.tree = tree

		last, err := h.blocks.Last()
		if err != nil {
			return nil, xerrors.Errorf("couldn't get latest block: %v", err)
		}

		for _, res := range last.GetTo().GetData().GetTransactionResults() {
			h.pool.Remove(res.GetTransaction())
		}

		h.watcher.Notify(ordering.Event{Index: last.GetTo().GetIndex()})
	case types.ViewMessage:
		view := pbft.View{
			From:   req.Address,
			ID:     msg.GetID(),
			Leader: msg.GetLeader(),
		}

		h.pbftsm.Accept(view)
	default:
		return nil, xerrors.Errorf("unsupported message of type '%T'", req.Message)
	}

	return nil, nil
}

func (h *processor) getCurrentRoster() (viewchange.Authority, error) {
	data, err := h.tree.Get(keyRoster[:])
	if err != nil {
		return nil, xerrors.Errorf("read from tree: %v", err)
	}

	roster, err := h.rosterFac.AuthorityOf(h.context, data)
	if err != nil {
		return nil, xerrors.Errorf("decode failed: %v", err)
	}

	return roster, nil
}
