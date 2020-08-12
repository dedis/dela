package pbft

import (
	"context"
	"sync"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
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
	case PrePrepareState:
		return "preprepare"
	case PrepareState:
		return "prepare"
	case CommitState:
		return "commit"
	case ViewChangeState:
		return "viewchange"
	default:
		return "invalid"
	}
}

const (
	// InitialState is the entry state which means the beginning of the PBFT
	// protocol.
	InitialState State = iota

	// PrePrepareState is the state where the roster is assigned.
	PrePrepareState

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

// View is the view change request sent to other participants.
type View struct {
	From   mino.Address
	ID     types.Digest
	Leader int
}

// StateMachine is the interface to implement to support a PBFT protocol.
type StateMachine interface {
	GetState() State
	GetLeader() mino.Address
	PrePrepare(ro viewchange.Authority) error
	Prepare(block types.Block) (types.Digest, error)
	Commit(types.Digest, crypto.Signature) error
	Finalize(types.Digest, crypto.Signature) error
	Accept(View)
	AcceptAll([]View)
	Expire(addr mino.Address) (View, error)
	CatchUp(types.BlockLink) error
	Watch(context.Context) <-chan State
}

type round struct {
	leader     int
	threshold  int
	roster     viewchange.Authority
	id         types.Digest
	block      types.Block
	tree       hashtree.StagingTree
	prepareSig crypto.Signature
	views      map[mino.Address]struct{}
}

// AuthorityReader is a function to help the state machine to read the current
// authority for a given tree.
type AuthorityReader func(tree hashtree.Tree) (viewchange.Authority, error)

type pbftsm struct {
	sync.Mutex

	watcher     blockchain.Observable
	hashFac     crypto.HashFactory
	val         validation.Service
	verifierFac crypto.VerifierFactory
	blocks      blockstore.BlockStore
	genesis     blockstore.GenesisStore
	tree        blockstore.TreeCache
	authReader  AuthorityReader

	state State
	round round
}

// StateMachineParam is a structure to pass the different components of the PBFT
// state machine.
type StateMachineParam struct {
	Validation      validation.Service
	VerifierFactory crypto.VerifierFactory
	Blocks          blockstore.BlockStore
	Genesis         blockstore.GenesisStore
	Tree            blockstore.TreeCache
	AuthorityReader AuthorityReader
}

// NewStateMachine returns a new state machine.
func NewStateMachine(param StateMachineParam) StateMachine {
	return &pbftsm{
		watcher:     blockchain.NewWatcher(),
		hashFac:     crypto.NewSha256Factory(),
		val:         param.Validation,
		verifierFac: param.VerifierFactory,
		blocks:      param.Blocks,
		genesis:     param.Genesis,
		tree:        param.Tree,
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
func (m *pbftsm) GetLeader() mino.Address {
	m.Lock()
	defer m.Unlock()

	if m.round.roster == nil {
		return nil
	}

	iter := m.round.roster.AddressIterator()
	iter.Seek(m.round.leader)

	return iter.GetNext()
}

// PrePrepare implements pbft.StateMachine. If the state machine is in the
// initial state, it will set the roster and moves to the PrePrepare state,
// otherwise it will return an error.
func (m *pbftsm) PrePrepare(ro viewchange.Authority) error {
	m.Lock()
	defer m.Unlock()

	if m.state != InitialState {
		// The roster can change only at the beginning of a PBFT round.
		return xerrors.Errorf("mismatch state %v != %v", m.state, InitialState)
	}

	m.round.leader = 0
	m.round.roster = ro
	// In case of view change, it requires 2f messages from other participants.
	m.round.threshold = calculateThreshold(ro.Len())

	m.setState(PrePrepareState)

	return nil
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

	if m.state != PrePrepareState {
		return types.Digest{}, xerrors.Errorf("mismatch state %v != %v", m.state, PrePrepareState)
	}

	err := m.verifyPrepare(m.tree.Get(), block, &m.round)
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

	err := m.verifyCommit(&m.round, sig)
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

	err := m.verifyFinalize(&m.round, sig)
	if err != nil {
		return err
	}

	m.round = round{
		roster: m.round.roster,
	}

	m.setState(InitialState)

	return nil
}

// Accept implements pbft.StateMachine. It processes view change messages and
// reset the current PBFT if it receives enough of them.
func (m *pbftsm) Accept(view View) {
	m.Lock()
	defer m.Unlock()

	if view.Leader != m.round.leader+1 {
		// The state machine ignore view messages from different rounds. It only
		// accepts views for the next leader even if the state machine is not in
		// ViewChange state.
		return
	}

	if view.ID != m.round.id {
		dela.Logger.Debug().Msg("received a view for a different block")
		return
	}

	if m.round.views == nil {
		m.round.views = make(map[mino.Address]struct{})
	}

	m.round.views[view.From] = struct{}{}

	m.checkViewChange(view)
}

// AcceptAll implements pbft.StateMachine. It accepts a list of views which
// allows a node falling behind to catch up. The list must contain enough views
// reach the threshold otherwise it will be ignored.
func (m *pbftsm) AcceptAll(views []View) {
	m.Lock()
	defer m.Unlock()

	set := make(map[mino.Address]struct{})

	for _, view := range views {
		if view.ID == m.round.id {
			set[view.From] = struct{}{}
		}
	}

	if len(views) <= m.round.threshold {
		// Not enough views to accept the view change.
		return
	}

	m.round.leader = views[0].Leader

	m.setState(PrePrepareState)
}

// Expire implements pbft.StateMachine. It moves the state machine to the
// ViewChange state. If it has already received enough messages, it will move to
// the next leader.
func (m *pbftsm) Expire(addr mino.Address) (View, error) {
	m.Lock()
	defer m.Unlock()

	view := View{
		From:   addr,
		Leader: m.round.leader + 1,
	}

	lastID, err := m.getLatestID()
	if err != nil {
		return view, xerrors.Errorf("couldn't get latest digest: %v", err)
	}

	view.ID = lastID

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
		roster:    m.round.roster,
		threshold: m.round.threshold,
	}

	err := m.verifyPrepare(m.tree.Get(), link.GetTo(), &r)
	if err != nil {
		return xerrors.Errorf("prepare failed: %v", err)
	}

	err = m.verifyCommit(&r, link.GetPrepareSignature())
	if err != nil {
		return xerrors.Errorf("commit failed: %v", err)
	}

	err = m.verifyFinalize(&r, link.GetCommitSignature())
	if err != nil {
		return xerrors.Errorf("finalize failed: %v", err)
	}

	m.round = round{
		roster: m.round.roster.Apply(link.GetChangeSet()),
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

func (m *pbftsm) verifyPrepare(tree hashtree.Tree, block types.Block, r *round) error {
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

	link := types.NewBlockLink(lastID, block, nil, nil, nil)

	h := m.hashFac.New()
	err = link.Fingerprint(h)
	if err != nil {
		return xerrors.Errorf("couldn't fingerprint link: %v", err)
	}

	copy(r.id[:], h.Sum(nil))

	r.tree = stageTree
	r.block = block

	return nil
}

func (m *pbftsm) verifyCommit(r *round, sig crypto.Signature) error {
	verifier, err := m.verifierFac.FromAuthority(r.roster)
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

func (m *pbftsm) verifyFinalize(r *round, sig crypto.Signature) error {
	verifier, err := m.verifierFac.FromAuthority(r.roster)
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

	roster, err := m.authReader(r.tree)
	if err != nil {
		return xerrors.Errorf("read roster failed: %v", err)
	}

	tree, err := r.tree.Commit()
	if err != nil {
		// TODO: database tx
		return err
	}

	m.tree.Set(tree)

	link := types.NewBlockLink(lastID, r.block, r.prepareSig, sig, r.roster.Diff(roster))

	err = m.blocks.Store(link)
	if err != nil {
		// TODO: database tx
		return err
	}

	return nil
}

func (m *pbftsm) setState(s State) {
	m.state = s
	m.watcher.Notify(s)
}

func (m *pbftsm) checkViewChange(view View) {
	if m.state == ViewChangeState && len(m.round.views) > m.round.threshold {
		m.round.leader = view.Leader
		m.setState(PrePrepareState)
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

	return last.GetTo().GetHash(), nil
}

type observer struct {
	ch chan State
}

func (obs observer) NotifyCallback(event interface{}) {
	obs.ch <- event.(State)
}

// CalculateThreshold returns the number of messages that a node needs to
// receive before confirming the view change. The threshold is 2*f where f can
// found with n = 3*f+1 where n is the number of participants.
func calculateThreshold(n int) int {
	f := (n - 1) / 3
	return 2 * f
}
