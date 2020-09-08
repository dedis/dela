package pbft

import (
	"context"
	"sync"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

// State is the type of the different possible states for the PBFT state
// machine.
type State byte

func (s State) String() string {
	switch s {
	case InitialState:
		return "initial"
	case PrepareState:
		return "prepare"
	case CommitState:
		return "commit"
	case ViewChangeState:
		return "viewchange"
	default:
		return "none"
	}
}

const (
	// InitialState is the entry state which means the beginning of the PBFT
	// protocol.
	InitialState State = iota

	// PrepareState is the state to indicate that a proposal has been received
	// and the machine is waiting for confirmations.
	PrepareState

	// CommitState is the state to indicate that the participants have accepted
	// the proposal and the machine is waiting for the second confirmation.
	CommitState

	// ViewChangeState is the state to indicate that the protocol is failing and
	// the machine is waiting for view change requests.
	ViewChangeState
)

// StateMachine is the interface to implement to support a PBFT protocol.
type StateMachine interface {
	GetState() State
	GetLeader() (mino.Address, error)
	Prepare(block types.Block) (types.Digest, error)
	Commit(types.Digest, crypto.Signature) error
	Finalize(types.Digest, crypto.Signature) error
	Accept(View) error
	AcceptAll([]View) error
	Expire(addr mino.Address) (View, error)
	CatchUp(types.BlockLink) error
	Watch(context.Context) <-chan State
}

type round struct {
	leader     uint16
	threshold  int
	id         types.Digest
	block      types.Block
	tree       hashtree.StagingTree
	prepareSig crypto.Signature
	changeset  authority.ChangeSet
	views      map[mino.Address]struct{}
}

// AuthorityReader is a function to help the state machine to read the current
// authority for a given tree.
type AuthorityReader func(tree hashtree.Tree) (authority.Authority, error)

type pbftsm struct {
	sync.Mutex

	logger     zerolog.Logger
	watcher    core.Observable
	hashFac    crypto.HashFactory
	val        validation.Service
	blocks     blockstore.BlockStore
	genesis    blockstore.GenesisStore
	tree       blockstore.TreeCache
	authReader AuthorityReader
	db         kv.DB

	// verifierFac creates a verifier for the aggregated signature.
	verifierFac crypto.VerifierFactory
	// signer signs and verify single signature for the view change.
	signer crypto.Signer

	state State
	round round
}

// StateMachineParam is a structure to pass the different components of the PBFT
// state machine.
type StateMachineParam struct {
	Validation      validation.Service
	VerifierFactory crypto.VerifierFactory
	Signer          crypto.Signer
	Blocks          blockstore.BlockStore
	Genesis         blockstore.GenesisStore
	Tree            blockstore.TreeCache
	AuthorityReader AuthorityReader
	DB              kv.DB
}

// NewStateMachine returns a new state machine.
func NewStateMachine(param StateMachineParam) StateMachine {
	return &pbftsm{
		logger:      dela.Logger,
		watcher:     core.NewWatcher(),
		hashFac:     crypto.NewSha256Factory(),
		val:         param.Validation,
		verifierFac: param.VerifierFactory,
		signer:      param.Signer,
		blocks:      param.Blocks,
		genesis:     param.Genesis,
		tree:        param.Tree,
		db:          param.DB,
		state:       InitialState,
		authReader:  param.AuthorityReader,
	}
}

// GetState implements pbft.StateMachine. It returns the current state of the
// machine.
func (m *pbftsm) GetState() State {
	m.Lock()
	defer m.Unlock()

	return m.state
}

// GetLeader implements pbft.StateMachine. It returns the current leader of the
// round, or nil if the roster is not yet defined.
func (m *pbftsm) GetLeader() (mino.Address, error) {
	m.Lock()
	defer m.Unlock()

	roster, err := m.authReader(m.tree.Get())
	if err != nil {
		return nil, xerrors.Errorf("failed to read roster: %v", err)
	}

	iter := roster.AddressIterator()
	iter.Seek(int(m.round.leader))

	return iter.GetNext(), nil
}

// Prepare implements pbft.StateMachine. It receives the proposal from the
// leader and the current tree, and produces the next tree alongside the ID of
// the proposal that will be signed.
func (m *pbftsm) Prepare(block types.Block) (types.Digest, error) {
	m.Lock()
	defer m.Unlock()

	if m.state == PrepareState {
		// It only accepts one proposal from the leader and skip any more
		// arriving.
		return m.round.id, nil
	}

	if m.state != InitialState {
		return types.Digest{}, xerrors.Errorf("mismatch state %v != %v", m.state, InitialState)
	}

	roster, err := m.authReader(m.tree.Get())
	if err != nil {
		return types.Digest{}, xerrors.Errorf("failed to read roster: %v", err)
	}

	m.round.threshold = calculateThreshold(roster.Len())

	err = m.verifyPrepare(m.tree.Get(), block, &m.round, roster)
	if err != nil {
		return types.Digest{}, err
	}

	m.setState(PrepareState)

	return m.round.id, nil
}

// Commit implements pbft.StateMachine. It commits the state machine to the
// proposal if the signature is verified.
func (m *pbftsm) Commit(id types.Digest, sig crypto.Signature) error {
	m.Lock()
	defer m.Unlock()

	if m.state == CommitState {
		// The state machine is already committed to a proposal.
		return nil
	}

	if m.state != PrepareState {
		return xerrors.Errorf("mismatch state %v != %v", m.state, PrepareState)
	}

	if id != m.round.id {
		return xerrors.Errorf("mismatch id '%v' != '%v'", id, m.round.id)
	}

	roster, err := m.authReader(m.tree.Get())
	if err != nil {
		return xerrors.Errorf("failed to read roster: %v", err)
	}

	err = m.verifyCommit(&m.round, sig, roster)
	if err != nil {
		return err
	}

	m.setState(CommitState)

	return nil
}

// Finalize implements pbft.StateMachine. It makes sure the commit signature is
// correct and then moves to the initial state.
func (m *pbftsm) Finalize(id types.Digest, sig crypto.Signature) error {
	m.Lock()
	defer m.Unlock()

	if m.state != CommitState {
		return xerrors.Errorf("mismatch state %v != %v", m.state, CommitState)
	}

	roster, err := m.authReader(m.tree.Get())
	if err != nil {
		return xerrors.Errorf("failed to read roster: %v", err)
	}

	err = m.verifyFinalize(&m.round, sig, roster)
	if err != nil {
		return err
	}

	m.setState(InitialState)

	return nil
}

// Accept implements pbft.StateMachine. It processes view change messages and
// reset the current PBFT if it receives enough of them.
func (m *pbftsm) Accept(view View) error {
	m.Lock()
	defer m.Unlock()

	err := m.verifyViews(false, view)
	if err != nil {
		return xerrors.Errorf("invalid view: %v", err)
	}

	if m.round.views == nil {
		m.round.views = make(map[mino.Address]struct{})
	}

	m.round.views[view.from] = struct{}{}

	m.checkViewChange(view)

	return nil
}

// AcceptAll implements pbft.StateMachine. It accepts a list of views which
// allows a node falling behind to catch up. The list must contain enough views
// reach the threshold otherwise it will be ignored.
func (m *pbftsm) AcceptAll(views []View) error {
	m.Lock()
	defer m.Unlock()

	if len(views) <= m.round.threshold {
		return xerrors.New("not enough views")
	}

	err := m.verifyViews(true, views...)
	if err != nil {
		return xerrors.Errorf("invalid view: %v", err)
	}

	set := make(map[mino.Address]struct{})

	for _, view := range views {
		if view.id == m.round.id {
			set[view.from] = struct{}{}
		}
	}

	m.round.views = set
	m.state = ViewChangeState
	m.checkViewChange(views[0])

	return nil
}

func (m *pbftsm) verifyViews(skip bool, views ...View) error {
	roster, err := m.authReader(m.tree.Get())
	if err != nil {
		return xerrors.Errorf("failed to read roster: %v", err)
	}

	for _, view := range views {
		pubkey, _ := roster.GetPublicKey(view.from)
		if pubkey == nil {
			return xerrors.Errorf("unknown peer: %v", view.from)
		}

		err := view.Verify(pubkey)
		if err != nil {
			return xerrors.Errorf("invalid signature: %v", err)
		}

		if !skip && view.leader != m.round.leader+1 {
			// The state machine ignore view messages from different rounds. It only
			// accepts views for the next leader even if the state machine is not in
			// ViewChange state.
			//
			// This check can be skipped when enough views are received for a
			// given leader index.
			return xerrors.Errorf("mismatch leader %d != %d", view.leader, m.round.leader+1)
		}

		if view.id != m.round.id {
			return xerrors.Errorf("mismatch id %v != %v", view.id, m.round.id)
		}
	}

	return nil
}

// Expire implements pbft.StateMachine. It moves the state machine to the
// ViewChange state. If it has already received enough messages, it will move to
// the next leader.
func (m *pbftsm) Expire(addr mino.Address) (View, error) {
	m.Lock()
	defer m.Unlock()

	lastID, err := m.getLatestID()
	if err != nil {
		return View{}, xerrors.Errorf("couldn't get latest digest: %v", err)
	}

	param := ViewParam{
		From:   addr,
		ID:     lastID,
		Leader: m.round.leader + 1,
	}

	view, err := NewViewAndSign(param, m.signer)
	if err != nil {
		return view, xerrors.Errorf("create view: %v", err)
	}

	m.setState(ViewChangeState)

	if m.round.views == nil {
		m.round.views = make(map[mino.Address]struct{})
	}

	m.round.views[addr] = struct{}{}

	m.checkViewChange(view)

	return view, nil
}

// CatchUp implements pbft.StateMachine. It can force a block link to be
// inserted to reset the state machine to a initial state.
func (m *pbftsm) CatchUp(link types.BlockLink) error {
	m.Lock()
	defer m.Unlock()

	r := round{
		threshold: m.round.threshold,
	}

	roster, err := m.authReader(m.tree.Get())
	if err != nil {
		return xerrors.Errorf("failed to read roster: %v", err)
	}

	err = m.verifyPrepare(m.tree.Get(), link.GetBlock(), &r, roster)
	if err != nil {
		return xerrors.Errorf("prepare failed: %v", err)
	}

	err = m.verifyCommit(&r, link.GetPrepareSignature(), roster)
	if err != nil {
		return xerrors.Errorf("commit failed: %v", err)
	}

	err = m.verifyFinalize(&r, link.GetCommitSignature(), roster)
	if err != nil {
		return xerrors.Errorf("finalize failed: %v", err)
	}

	m.setState(InitialState)

	return nil
}

// Watch implements pbft.StateMachine. It returns a channel that will be
// populated with stage changes.
func (m *pbftsm) Watch(ctx context.Context) <-chan State {
	ch := make(chan State, 100)

	obs := observer{ch: ch}
	m.watcher.Add(obs)

	go func() {
		<-ctx.Done()
		m.watcher.Remove(obs)
		close(ch)
	}()

	return ch
}

func (m *pbftsm) verifyPrepare(tree hashtree.Tree, block types.Block, r *round, ro authority.Authority) error {
	stageTree, err := tree.Stage(func(snap store.Snapshot) error {
		_, err := m.val.Validate(snap, block.GetTransactions())
		if err != nil {
			return xerrors.Errorf("validation failed: %v", err)
		}

		return nil
	})

	if err != nil {
		return xerrors.Errorf("tree failed: %v", err)
	}

	root := types.Digest{}
	copy(root[:], stageTree.GetRoot())

	if root != block.GetTreeRoot() {
		return xerrors.Errorf("mismatch tree root '%v' != '%v'", root, block.GetTreeRoot())
	}

	if m.blocks.Len()+1 != block.GetIndex() {
		return xerrors.Errorf("mismatch index %d != %d", block.GetIndex(), m.blocks.Len()+1)
	}

	lastID, err := m.getLatestID()
	if err != nil {
		return xerrors.Errorf("couldn't get latest digest: %v", err)
	}

	// The roster will be used to find the differential with the previous one so
	// that the forward link can be populated.
	roster, err := m.authReader(stageTree)
	if err != nil {
		return xerrors.Errorf("failed to read next roster: %v", err)
	}

	changeset := ro.Diff(roster)
	opts := []types.LinkOption{
		types.WithChangeSet(changeset),
		types.WithLinkHashFactory(m.hashFac),
	}

	link, err := types.NewForwardLink(lastID, block.GetHash(), opts...)
	if err != nil {
		return xerrors.Errorf("failed to create link: %v", err)
	}

	r.id = link.GetHash()
	r.tree = stageTree
	r.block = block
	r.changeset = changeset

	return nil
}

func (m *pbftsm) verifyCommit(r *round, sig crypto.Signature, ro authority.Authority) error {
	verifier, err := m.verifierFac.FromAuthority(ro)
	if err != nil {
		return xerrors.Errorf("couldn't make verifier: %v", err)
	}

	err = verifier.Verify(r.id[:], sig)
	if err != nil {
		return xerrors.Errorf("verifier failed: %v", err)
	}

	r.prepareSig = sig

	return nil
}

func (m *pbftsm) verifyFinalize(r *round, sig crypto.Signature, ro authority.Authority) error {
	verifier, err := m.verifierFac.FromAuthority(ro)
	if err != nil {
		return xerrors.Errorf("couldn't make verifier: %v", err)
	}

	buffer, err := r.prepareSig.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("couldn't marshal signature: %v", err)
	}

	err = verifier.Verify(buffer, sig)
	if err != nil {
		return xerrors.Errorf("verifier failed: %v", err)
	}

	lastID, err := m.getLatestID()
	if err != nil {
		return xerrors.Errorf("couldn't get latest digest: %v", err)
	}

	// Persist to the database in a transaction so that it can revert to the
	// previous state for either the tree or the block if something goes wrong.
	err = m.db.Update(func(txn kv.WritableTx) error {
		// 1. Persist the tree through the transaction and update the cache.
		err := r.tree.WithTx(txn).Commit()
		if err != nil {
			return xerrors.Errorf("commit tree: %v", err)
		}

		txn.OnCommit(func() {
			// The cache is updated only after both are committed with the tree
			// using the database as the transaction is done.
			m.tree.Set(r.tree)
		})

		// 2. Persist the block and its forward link.
		opts := []types.LinkOption{
			types.WithSignatures(r.prepareSig, sig),
			types.WithChangeSet(r.changeset),
			types.WithLinkHashFactory(m.hashFac),
		}

		link, err := types.NewBlockLink(lastID, r.block, opts...)
		if err != nil {
			return xerrors.Errorf("creating link: %v", err)
		}

		err = m.blocks.WithTx(txn).Store(link)
		if err != nil {
			return xerrors.Errorf("store block: %v", err)
		}

		return nil
	})

	if err != nil {
		return xerrors.Errorf("database failed: %v", err)
	}

	return nil
}

func (m *pbftsm) setState(s State) {
	m.state = s
	m.watcher.Notify(s)
}

func (m *pbftsm) checkViewChange(view View) {
	if m.state == ViewChangeState && len(m.round.views) > m.round.threshold {
		m.round.leader = view.leader
		m.setState(InitialState)
	}
}

func (m *pbftsm) getLatestID() (types.Digest, error) {
	if m.blocks.Len() == 0 {
		genesis, err := m.genesis.Get()
		if err != nil {
			return types.Digest{}, err
		}

		return genesis.GetHash(), nil
	}

	last, err := m.blocks.Last()
	if err != nil {
		return types.Digest{}, err
	}

	return last.GetTo(), nil
}

type observer struct {
	ch chan State
}

func (obs observer) NotifyCallback(event interface{}) {
	obs.ch <- event.(State)
}

// CalculateThreshold returns the number of messages that a node needs to
// receive before confirming the view change. The threshold is 2*f where f can
// be found with n = 3*f+1 where n is the number of participants.
func calculateThreshold(n int) int {
	f := (n - 1) / 3
	return 2 * f
}
