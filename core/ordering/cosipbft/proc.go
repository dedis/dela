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

type processor struct {
	mino.UnsupportedHandler
	types.MessageFactory

	committer   pbft.StateMachine
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

func (h *processor) Invoke(from mino.Address, msg serde.Message) ([]byte, error) {
	switch in := msg.(type) {
	case types.BlockMessage:
		if h.committer.GetState() == pbft.InitialState {
			roster, err := h.getCurrentRoster()
			if err != nil {
				return nil, err
			}

			err = h.committer.PrePrepare(roster)
			if err != nil {
				return nil, err
			}
		}

		digest, err := h.committer.Prepare(in.GetBlock(), h.tree)
		if err != nil {
			return nil, xerrors.Errorf("committer failed: %v", err)
		}

		return digest[:], nil
	case types.CommitMessage:
		err := h.committer.Commit(in.GetID(), in.GetSignature())
		if err != nil {
			return nil, err
		}

		buffer, err := in.GetSignature().MarshalBinary()
		if err != nil {
			return nil, err
		}

		return buffer, nil
	default:
		return nil, xerrors.New("unsupported message")
	}
}

func (h *processor) Process(req mino.Request) (serde.Message, error) {
	switch msg := req.Message.(type) {
	case types.GenesisMessage:
		roster, err := msg.GetGenesis().GetRoster().Serialize(h.context)
		if err != nil {
			return nil, err
		}

		stageTree, err := h.tree.Stage(func(snap store.Snapshot) error {
			snap.Set(keyRoster[:], roster)
			return nil
		})
		if err != nil {
			return nil, err
		}

		tree, err := stageTree.Commit()
		if err != nil {
			return nil, err
		}

		h.tree = tree
		err = h.genesis.Set(*msg.GetGenesis())
		if err != nil {
			return nil, err
		}

		h.watcher.Notify(ordering.Event{Index: 0})

		close(h.started)
	case types.DoneMessage:
		tree, err := h.committer.Finalize(msg.GetID(), msg.GetSignature())
		if err != nil {
			return nil, err
		}

		h.tree = tree

		last, err := h.blocks.Last()
		if err != nil {
			return nil, err
		}

		for _, res := range last.GetTo().GetData().GetTransactionResults() {
			h.pool.Remove(res.GetTransaction())
		}

		h.watcher.Notify(ordering.Event{Index: 1})
	case types.ViewMessage:
		view := pbft.View{
			From:   req.Address,
			ID:     msg.GetID(),
			Leader: msg.GetLeader(),
		}

		h.committer.Accept(view)
	default:
		return nil, xerrors.Errorf("unsupported message '%T'", req.Message)
	}

	return nil, nil
}

func (h *processor) getCurrentRoster() (viewchange.Authority, error) {
	data, err := h.tree.Get(keyRoster[:])
	if err != nil {
		return nil, err
	}

	roster, err := h.rosterFac.AuthorityOf(h.context, data)
	if err != nil {
		return nil, err
	}

	return roster, nil
}
