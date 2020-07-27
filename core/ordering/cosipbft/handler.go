package cosipbft

import (
	"bytes"

	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/tap/pool"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
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

	committer   TwoPhaseCommit
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

func newHandler(
	tree hashtree.Tree,
	rf viewchange.AuthorityFactory,
	c TwoPhaseCommit,
	hf crypto.HashFactory,
	gs blockstore.GenesisStore,
	bs blockstore.BlockStore,
	p pool.Pool,
) *handler {

	return &handler{
		committer:   c,
		tree:        tree,
		watcher:     blockchain.NewWatcher(),
		rosterFac:   rf,
		hashFactory: hf,
		context:     json.NewContext(),
		started:     make(chan struct{}),
		genesis:     gs,
		blocks:      bs,
		pool:        p,
	}
}

func (h *handler) Invoke(from mino.Address, msg serde.Message) ([]byte, error) {
	switch in := msg.(type) {
	case types.BlockMessage:
		digest, err := h.committer.Propose(in.GetBlock(), h.tree)
		if err != nil {
			return nil, xerrors.Errorf("committer failed: %v", err)
		}

		return digest, nil
	case types.CommitMessage:
		roster, err := h.getCurrentRoster()
		if err != nil {
			return nil, err
		}

		err = h.committer.Commit(in.GetID(), in.GetSignature(), roster)
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
		err = h.genesis.Set(*msg.GetGenesis())
		if err != nil {
			return nil, err
		}

		h.watcher.Notify(ordering.Event{Index: 0})

		close(h.started)
	case types.DoneMessage:
		roster, err := h.getCurrentRoster()
		if err != nil {
			return nil, err
		}

		tree, err := h.committer.Finalize(msg.GetID(), msg.GetSignature(), roster)
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

// TwoPhaseCommit is an interface to manage the different phases of a CoSiPBFT
// protocol.
type TwoPhaseCommit interface {
	Propose(block types.Block, tree hashtree.Tree) ([]byte, error)
	Commit(id []byte, sig crypto.Signature, roster viewchange.Authority) error
	Finalize(id []byte, sig crypto.Signature, roster viewchange.Authority) (hashtree.Tree, error)
}

type proposal struct {
	block      types.Block
	tree       hashtree.StagingTree
	prepareSig crypto.Signature
}

type default2PC struct {
	props       map[string]*proposal
	val         validation.Service
	cosi        cosi.CollectiveSigning
	hashFactory crypto.HashFactory
	blocks      blockstore.BlockStore
	genesis     blockstore.GenesisStore
}

func newTwoPhaseCommit(
	val validation.Service,
	c cosi.CollectiveSigning,
	hf crypto.HashFactory,
	gs blockstore.GenesisStore,
	bs blockstore.BlockStore,
) *default2PC {
	return &default2PC{
		props:       make(map[string]*proposal),
		val:         val,
		cosi:        c,
		hashFactory: hf,
		blocks:      bs,
		genesis:     gs,
	}
}

func (c *default2PC) Propose(block types.Block, tree hashtree.Tree) ([]byte, error) {
	stageTree, err := tree.Stage(func(snap store.Snapshot) error {
		_, err := c.val.Validate(snap, block.GetTransactions())
		return err
	})

	if err != nil {
		return nil, xerrors.Errorf("validation failed: %v", err)
	}

	if !bytes.Equal(stageTree.GetRoot(), block.GetTreeRoot()) {
		return nil, xerrors.Errorf("mismatch tree root")
	}

	lastID, err := c.getLatestID()
	if err != nil {
		return nil, err
	}

	link := types.NewForwardLink(lastID, block)

	h := c.hashFactory.New()
	err = link.Fingerprint(h)
	if err != nil {
		return nil, err
	}

	id := h.Sum(nil)

	c.props[string(id)] = &proposal{
		block: block,
		tree:  stageTree,
	}

	return id, nil
}

func (c *default2PC) Commit(id []byte, sig crypto.Signature, roster viewchange.Authority) error {
	prop, found := c.props[string(id)]
	if !found {
		return xerrors.New("proposal not found")
	}

	verifier, err := c.cosi.GetVerifierFactory().FromAuthority(roster)
	if err != nil {
		return err
	}

	err = verifier.Verify(id, sig)
	if err != nil {
		return err
	}

	prop.prepareSig = sig

	return nil
}

func (c *default2PC) Finalize(id []byte, sig crypto.Signature, roster viewchange.Authority) (hashtree.Tree, error) {
	prop, found := c.props[string(id)]
	if !found {
		return nil, xerrors.New("proposal not found")
	}

	if prop.prepareSig == nil {
		return nil, xerrors.Errorf("missing prepare signature")
	}

	verifier, err := c.cosi.GetVerifierFactory().FromAuthority(roster)
	if err != nil {
		return nil, err
	}

	buffer, err := prop.prepareSig.MarshalBinary()
	if err != nil {
		return nil, err
	}

	err = verifier.Verify(buffer, sig)
	if err != nil {
		return nil, err
	}

	lastID, err := c.getLatestID()
	if err != nil {
		return nil, err
	}

	link := types.NewForwardLink(lastID, prop.block)

	err = c.blocks.Store(link)
	if err != nil {
		// TODO: database tx
		return nil, err
	}

	c.props = make(map[string]*proposal)

	tree, err := prop.tree.Commit()
	if err != nil {
		// TODO: database tx
		return nil, err
	}

	return tree, nil
}

func (c *default2PC) getLatestID() ([]byte, error) {
	if c.blocks.Len() == 0 {
		genesis, err := c.genesis.Get()
		if err != nil {
			return nil, err
		}

		return genesis.GetHash(), nil
	}

	last, err := c.blocks.Last()
	if err != nil {
		return nil, err
	}

	return last.GetTo().GetHash(), nil
}
