package cosipbft

import (
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

var (
	keyRoster = [32]byte{0}
)

type handler struct {
	mino.UnsupportedHandler
	types.MessageFactory

	tree       hashtree.Tree
	validation validation.Service
	rosterFac  viewchange.AuthorityFactory
	watcher    blockchain.Observable
	context    serde.Context

	genesis *types.Genesis

	started chan struct{}
}

func newHandler(tree hashtree.Tree, val validation.Service, rf viewchange.AuthorityFactory) *handler {
	return &handler{
		tree:       tree,
		validation: val,
		rosterFac:  rf,
		watcher:    blockchain.NewWatcher(),
		context:    json.NewContext(),
		started:    make(chan struct{}),
	}
}

func (h *handler) Invoke(from mino.Address, msg serde.Message) ([]byte, error) {
	switch msg.(type) {
	case types.BlockMessage:
		return []byte{}, nil
	case types.CommitMessage:
		return []byte{}, nil
	default:
		return nil, xerrors.New("unsupported message")
	}
}

func (h *handler) Process(req mino.Request) (serde.Message, error) {
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
		h.genesis = msg.GetGenesis()

		h.watcher.Notify(ordering.Event{Index: 0})

		close(h.started)
	case types.DoneMessage:
		h.watcher.Notify(ordering.Event{Index: 1})
	default:
		return nil, xerrors.Errorf("unsupported message '%T'", req.Message)
	}

	return nil, nil
}

func (h *handler) getCurrentRoster() (viewchange.Authority, error) {
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
