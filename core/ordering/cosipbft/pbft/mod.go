// Package pbft defines a state machine to perform PBFT using collective
// signatures.
//
// The package also implements a default state machine that allows only one
// block candidate per leader so that after a successful prepare phase, it
// expects the block to be committed and finalized. The only other state allowed
// is the view change if the round has expired.
//
// The view change can be fixed only by providing enough valid views from unique
// participants to comply to the 2f threshold, or if a catch up that provides a
// proof of acceptance of the block.
//
// Documentation Last Review: 13.10.2020
//
package pbft

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
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
	case NoneState:
		return "none"
	case InitialState:
		return "initial"
	case PrepareState:
		return "prepare"
	case CommitState:
		return "commit"
	case ViewChangeState:
		return "viewchange"
	default:
		return "unknown"
	}
}

// defines prometheus metrics
var (
	promBlocks = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dela_cosipbft_blocks_total",
		Help: "total number of blocks",
	})

	promTxs = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "dela_cosipbft_transactions_block",
		Help:    "total number of transactions in the last block",
		Buckets: []float64{0, 1, 2, 3, 5, 8, 13, 20, 30, 50, 100},
	})

	promRejectedTxs = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "dela_cosipbft_transactions_rejected_block",
		Help:    "total number of rejected transactions in the last block",
		Buckets: []float64{0, 1, 2, 3, 5, 8, 13, 20, 30, 50, 100},
	})

	promLeader = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dela_cosipbft_leader",
		Help: "leader index from the roster",
	})
)

const (
	// NoneState is the very first state of the machine where nothing is set.
	NoneState State = iota

	// InitialState is the entry state which means the beginning of the PBFT
	// protocol.
	InitialState

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

func init() {
	dela.PromCollectors = append(dela.PromCollectors, promBlocks, promTxs,
		promRejectedTxs, promLeader)
}

// StateMachine is the interface to implement to support a PBFT protocol.
type StateMachine interface {
	// GetState returns the current state.
	GetState() State

	// GetLeader returns the address of the round leader.
	GetLeader() (mino.Address, error)

	// GetViews returns the list of views for which the round has been accepted,
	// after a successful view change. Otherwise it is empty.
	GetViews() map[mino.Address]View

	// GetCommit returns the candidate digest and the associated block if the
	// state machine is committed to a candidate, otherwise the behaviour is
	// undefined.
	GetCommit() (types.Digest, types.Block)

	// Prepare processes the candidate block and moves the state machine if it
	// is valid and from the correct leader.
	Prepare(from mino.Address, block types.Block) (types.Digest, error)

	// Commit moves the state machine to the next state if the signature is
	// valid for the candidate.
	Commit(types.Digest, crypto.Signature) error

	// Finalize finalizes a round if the signature is a valid commit signature.
	Finalize(types.Digest, crypto.Signature) error

	// Accept processes the view during a view change state, and moves to a new
	// round if enough have been received.
	Accept(View) error

	// AcceptAll processes the list of views so that it may proceed to a future
	// round if the list contains enough valid views.
	AcceptAll([]View) error

	// Expire announces that the round has expired and moves the state machine
	// to a view change state.
	Expire(addr mino.Address) (View, error)

	// CatchUp forces a valid block to be processed by the state machine without
	// doing the intermediate phases.
	CatchUp(types.BlockLink) error

	// Watch returns a channel that is populated with the changes of states from
	// the state machine.
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
	committed  bool
	prevViews  map[mino.Address]View
	views      map[mino.Address]View

	// allows a node to catch up on a new leader
	tentativeRound  types.Digest
	tentativeLeader uint16
}

// AuthorityReader is a function to help the state machine to read the current
// authority for a given tree.
type AuthorityReader func(tree hashtree.Tree) (authority.Authority, error)

// pbftsm is an implementation of a state machine to perform PBFT rounds.
//
// - implements pbft.Statemachine
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
	Logger          zerolog.Logger
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
		logger:      param.Logger,
		watcher:     core.NewWatcher(),
		hashFac:     crypto.NewSha256Factory(),
		val:         param.Validation,
		verifierFac: param.VerifierFactory,
		signer:      param.Signer,
		blocks:      param.Blocks,
		genesis:     param.Genesis,
		tree:        param.Tree,
		db:          param.DB,
		state:       NoneState,
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

// GetViews implements pbft.StateMachine. It returns the views for which the
// current round has been accepted.
func (m *pbftsm) GetViews() map[mino.Address]View {
	m.Lock()

	views := make(map[mino.Address]View)
	for key, value := range m.round.prevViews {
		views[key] = value
	}

	m.Unlock()

	return views
}

// GetCommit implements pbft.StateMachine. It returns the proposal identifier
// and the block that have been proposed to the state machine. The values are
// valid only if the state is at least PrepareState.
func (m *pbftsm) GetCommit() (types.Digest, types.Block) {
	m.Lock()
	defer m.Unlock()

	return m.round.id, m.round.block
}

// Prepare implements pbft.StateMachine. It receives the proposal from the
// leader and the current tree, and produces the next tree alongside the ID of
// the proposal that will be signed.
func (m *pbftsm) Prepare(from mino.Address, block types.Block) (types.Digest, error) {
	m.Lock()
	defer m.Unlock()

	id := m.round.id

	if m.state == ViewChangeState {
		// When in view change mode, it must refuse any proposal incoming until
		// the node leaves the state.
		return id, xerrors.New("cannot be in view change state during prepare")
	}

	roster, err := m.authReader(m.tree.Get())
	if err != nil {
		return id, xerrors.Errorf("failed to read roster: %v", err)
	}

	_, index := roster.GetPublicKey(from)

	if uint16(index) != m.round.leader {
		// Allows the node to catchup on the leader. It rejects the proposal,
		// but will accept this leader later if the block is finalized and
		// synced to us.
		m.round.tentativeRound = block.GetHash()
		m.round.tentativeLeader = uint16(index)
		return id, xerrors.Errorf("'%v' is not the leader", from)
	}

	// Check the state after verifying that the proposal comes from the right
	// leader.
	if m.state == PrepareState || m.state == CommitState {
		// The leader should only propose one block, therefore the accepted
		// proposal identifier is sent back, whatever the input is.
		return id, nil
	}

	m.round.threshold = calculateThreshold(roster.Len())

	err = m.verifyPrepare(m.tree.Get(), block, &m.round, roster)
	if err != nil {
		return id, err
	}

	m.setState(PrepareState)

	return m.round.id, nil
}

// Commit implements pbft.StateMachine. It commits the state machine to the
// proposal if the signature is verified.
func (m *pbftsm) Commit(id types.Digest, sig crypto.Signature) error {
	m.Lock()
	defer m.Unlock()

	if m.state != PrepareState && m.state != CommitState {
		return xerrors.Errorf("cannot commit from %v state", m.state)
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

	// At this point, the proposal must be finalized whatever happens.
	m.round.committed = true

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

	dela.Logger.Info().Msgf("finalize round with leader: %d", m.round.leader)

	m.round.prevViews = nil
	m.round.views = nil
	m.round.committed = false

	m.setState(InitialState)

	return nil
}

// Accept implements pbft.StateMachine. It processes view change messages and
// reset the current PBFT if it receives enough of them.
func (m *pbftsm) Accept(view View) error {
	m.Lock()
	defer m.Unlock()

	_, err := m.init()
	if err != nil {
		return xerrors.Errorf("init: %v", err)
	}

	if view.leader == m.round.leader {
		// Ignore view coming for the current leader as we already accepted this
		// leader.
		return nil
	}

	err = m.verifyViews(false, view)
	if err != nil {
		return xerrors.Errorf("invalid view: %v", err)
	}

	if m.round.views == nil {
		m.round.views = make(map[mino.Address]View)
	}

	m.logger.Trace().
		Str("from", view.from.String()).
		Msg("view accepted")

	m.round.views[view.from] = view

	m.checkViewChange(view)

	return nil
}

// AcceptAll implements pbft.StateMachine. It accepts a list of views which
// allows a node falling behind to catch up. The list must contain enough views
// to reach the threshold, otherwise it will be ignored.
func (m *pbftsm) AcceptAll(views []View) error {
	m.Lock()
	defer m.Unlock()

	_, err := m.init()
	if err != nil {
		return xerrors.Errorf("init: %v", err)
	}

	if len(views) <= m.round.threshold {
		return xerrors.Errorf("not enough views: %d <= %d",
			len(views), m.round.threshold)
	}

	if views[0].leader == m.round.leader {
		// Skip verifying the views if the leader will anyway be the same.
		return nil
	}

	err = m.verifyViews(true, views...)
	if err != nil {
		return xerrors.Errorf("invalid view: %v", err)
	}

	set := make(map[mino.Address]View)
	for _, view := range views {
		set[view.from] = view
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

		nextLeader := (m.round.leader + 1) % uint16(roster.Len())
		if !skip && view.leader != nextLeader {
			// The state machine ignore view messages from different rounds. It only
			// accepts views for the next leader even if the state machine is not in
			// ViewChange state.
			//
			// This check can be skipped when enough views are received for a
			// given leader index.
			return xerrors.Errorf("mismatch leader %d != %d", view.leader, nextLeader)
		}

		latestID, err := m.getLatestID()
		if err != nil {
			return xerrors.Errorf("failed to read latest id: %v", err)
		}

		if view.id != latestID {
			return xerrors.Errorf("mismatch id %v != %v", view.id, latestID)
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

	m.logger.Info().Msgf("expire: current leader is %d", m.round.leader)

	roster, err := m.init()
	if err != nil {
		return View{}, xerrors.Errorf("init: %v", err)
	}

	lastID, err := m.getLatestID()
	if err != nil {
		return View{}, xerrors.Errorf("couldn't get latest digest: %v", err)
	}

	newLeader := (m.round.leader + 1) % uint16(roster.Len())

	param := ViewParam{
		From:   addr,
		ID:     lastID,
		Leader: newLeader,
	}

	view, err := NewViewAndSign(param, m.signer)
	if err != nil {
		return view, xerrors.Errorf("create view: %v", err)
	}

	m.setState(ViewChangeState)

	if m.round.views == nil {
		m.round.views = make(map[mino.Address]View)
	}

	m.round.views[addr] = view

	m.checkViewChange(view)

	return view, nil
}

// CatchUp implements pbft.StateMachine. It can force a block link to be
// inserted to reset the state machine to a initial state.
func (m *pbftsm) CatchUp(link types.BlockLink) error {
	m.Lock()
	defer m.Unlock()

	if m.state == CommitState && m.round.id != link.GetHash() {
		return xerrors.Errorf("already committed to '%v'", m.round.id)
	}

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

	if link.GetTo() == m.round.tentativeRound {
		m.round.leader = m.round.tentativeLeader
		dela.Logger.Info().Msgf("accepting to set leader to: %d", m.round.leader)
	}

	m.round.views = nil
	m.round.prevViews = nil
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
		txs := block.GetTransactions()
		rejected := 0

		res, err := m.val.Validate(snap, txs)
		if err != nil {
			return xerrors.Errorf("validation failed: %v", err)
		}

		for _, r := range res.GetTransactionResults() {
			accepted, reason := r.GetStatus()
			if !accepted {
				m.logger.Warn().Str("reason", reason).Msg("transaction not accepted")
				rejected++
			}
		}

		promTxs.Observe(float64(len(txs)))
		promRejectedTxs.Observe(float64(rejected))

		return nil
	})

	if err != nil {
		return xerrors.Errorf("while updating tree: %v", err)
	}

	root := types.Digest{}
	copy(root[:], stageTree.GetRoot())

	if root != block.GetTreeRoot() {
		return xerrors.Errorf("mismatch tree root '%v' != '%v'", root, block.GetTreeRoot())
	}

	if m.blocks.Len() != block.GetIndex() {
		return xerrors.Errorf("mismatch index %d != %d", block.GetIndex(), m.blocks.Len())
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
			return xerrors.Errorf("while committing tree: %v", err)
		}

		var unlock func()

		txn.OnCommit(func() {
			// The cache is updated only after both are committed with the tree
			// using the database as the transaction is done.
			unlock = m.tree.SetWithLock(r.tree)
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

		// Only release the tree cache at the very end of the transaction, so
		// that a call to get the tree will hold until the block is stored.
		txn.OnCommit(func() {
			promBlocks.Set(float64(m.blocks.Len()))
			promLeader.Set(float64(m.round.leader))
			unlock()
		})

		return nil
	})

	if err != nil {
		return xerrors.Errorf("database failed: %v", err)
	}

	return nil
}

func (m *pbftsm) init() (authority.Authority, error) {
	roster, err := m.authReader(m.tree.Get())
	if err != nil {
		return nil, xerrors.Errorf("failed to read roster: %v", err)
	}

	if m.state != NoneState {
		return roster, nil
	}

	m.round.threshold = calculateThreshold(roster.Len())

	m.setState(InitialState)

	return roster, nil
}

func (m *pbftsm) setState(s State) {
	m.state = s
	m.watcher.Notify(s)
}

func (m *pbftsm) checkViewChange(view View) {
	if m.state == ViewChangeState && len(m.round.views) > m.round.threshold {
		m.round.prevViews = m.round.views
		m.round.views = nil
		m.round.leader = view.leader

		if m.round.committed {
			m.setState(CommitState)
		} else {
			m.setState(InitialState)
		}
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
	if f == 0 {
		return n
	}

	return 2 * f
}
