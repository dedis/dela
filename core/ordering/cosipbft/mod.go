// Package cosipbft implements an ordering service using collective signatures
// for the consensus.
//
// The consensus follows the PBFT algorithm using collective signatures to
// perform the prepare and commit phases. The leader is orchestrating the
// protocol and the followers wait for incoming messages to update their own
// state machines and reply with signatures when the leader candidate is valid.
// If the leader fails to send a candidate, or finalize it, the followers will
// timeout after some time and move to a view change state.
//
// The view change procedure is always waiting on the leader+1 confirmation
// before moving to leader+2, leader+3, etc. It means that if not enough nodes
// are online to create a block, the round will fail until enough wakes up and
// confirm leader+1. If leader+1 fails to create a block within the round
// timeout, a new view change starts for leader+2 and so on until a block is
// created.
//
// Before each PBFT round, a synchronization is run from the leader to allow
// nodes that have fallen behind (or are new) to catch missing blocks. Only a
// PBFT threshold of nodes needs to confirm a hard synchronization (having all
// the blocks) for the round to proceed, but others will keep catching up.
//
// Related Papers:
//
// Enhancing Bitcoin Security and Performance with Strong Consistency via
// Collective Signing (2016)
// https://www.usenix.org/system/files/conference/usenixsecurity16/sec16_paper_kokoris-kogias.pdf
//
// Documentation Last Review: 12.10.2020
//
package cosipbft

import (
	"context"
	"fmt"
	"math"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/blocksync"
	"go.dedis.ch/dela/core/ordering/cosipbft/contracts/viewchange"
	"go.dedis.ch/dela/core/ordering/cosipbft/pbft"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/cosi/threshold"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

const (
	// RoundTimeout is the maximum of time the service waits for an event to
	// happen.
	RoundTimeout = 10 * time.Second

	// RoundWait is the constant value of the exponential backoff use between
	// round failures.
	RoundWait = 5 * time.Millisecond

	// RoundMaxWait is the maximum amount for the backoff.
	RoundMaxWait = 5 * time.Minute

	rpcName = "cosipbft"
)

// RegisterRosterContract registers the native smart contract to update the
// roster to the given service.
func RegisterRosterContract(exec *native.Service, rFac authority.Factory, srvc access.Service) {
	contract := viewchange.NewContract(keyRoster[:], keyAccess[:], rFac, srvc)

	viewchange.RegisterContract(exec, contract)
}

// Service is an ordering service using collective signatures combined with PBFT
// to create a chain of blocks.
//
// - implements ordering.Service
type Service struct {
	*processor

	me          mino.Address
	rpc         mino.RPC
	actor       cosi.Actor
	val         validation.Service
	verifierFac crypto.VerifierFactory

	timeoutRound             time.Duration
	timeoutRoundAfterFailure time.Duration
	timeoutViewchange        time.Duration

	events      chan ordering.Event
	closing     chan struct{}
	closed      chan struct{}
	failedRound bool
}

type serviceTemplate struct {
	hashFac crypto.HashFactory
	blocks  blockstore.BlockStore
	genesis blockstore.GenesisStore
}

// ServiceOption is the type of option to set some fields of the service.
type ServiceOption func(*serviceTemplate)

// WithGenesisStore is an option to set the genesis store.
func WithGenesisStore(store blockstore.GenesisStore) ServiceOption {
	return func(tmpl *serviceTemplate) {
		tmpl.genesis = store
	}
}

// WithBlockStore is an option to set the block store.
func WithBlockStore(store blockstore.BlockStore) ServiceOption {
	return func(tmpl *serviceTemplate) {
		tmpl.blocks = store
	}
}

// WithHashFactory is an option to set the hash factory used by the service.
func WithHashFactory(fac crypto.HashFactory) ServiceOption {
	return func(tmpl *serviceTemplate) {
		tmpl.hashFac = fac
	}
}

// ServiceParam is the different components to provide to the service. All the
// fields are mandatory and it will panic if any is nil.
type ServiceParam struct {
	Mino       mino.Mino
	Cosi       cosi.CollectiveSigning
	Validation validation.Service
	Access     access.Service
	Pool       pool.Pool
	Tree       hashtree.Tree
	DB         kv.DB
}

// NewService starts a new ordering service.
func NewService(param ServiceParam, opts ...ServiceOption) (*Service, error) {
	tmpl := serviceTemplate{
		hashFac: crypto.NewSha256Factory(),
		genesis: blockstore.NewGenesisStore(),
		blocks:  blockstore.NewInMemory(),
	}

	for _, opt := range opts {
		opt(&tmpl)
	}

	proc := newProcessor()
	proc.hashFactory = tmpl.hashFac
	proc.blocks = tmpl.blocks
	proc.genesis = tmpl.genesis
	proc.pool = param.Pool
	proc.rosterFac = authority.NewFactory(param.Mino.GetAddressFactory(), param.Cosi.GetPublicKeyFactory())
	proc.tree = blockstore.NewTreeCache(param.Tree)
	proc.access = param.Access
	proc.logger = dela.Logger.With().Str("addr", param.Mino.GetAddress().String()).Logger()

	pcparam := pbft.StateMachineParam{
		Logger:          proc.logger,
		Validation:      param.Validation,
		Signer:          param.Cosi.GetSigner(),
		VerifierFactory: param.Cosi.GetVerifierFactory(),
		Blocks:          tmpl.blocks,
		Genesis:         tmpl.genesis,
		Tree:            proc.tree,
		AuthorityReader: proc.readRoster,
		DB:              param.DB,
	}

	proc.pbftsm = pbft.NewStateMachine(pcparam)

	blockFac := types.NewBlockFactory(param.Validation.GetFactory())
	csFac := authority.NewChangeSetFactory(param.Mino.GetAddressFactory(), param.Cosi.GetPublicKeyFactory())
	linkFac := types.NewLinkFactory(blockFac, param.Cosi.GetSignatureFactory(), csFac)
	chainFac := types.NewChainFactory(linkFac)

	syncparam := blocksync.SyncParam{
		Mino:            param.Mino,
		Blocks:          tmpl.blocks,
		Genesis:         tmpl.genesis,
		PBFT:            proc.pbftsm,
		LinkFactory:     linkFac,
		ChainFactory:    chainFac,
		VerifierFactory: param.Cosi.GetVerifierFactory(),
	}

	blocksync := blocksync.NewSynchronizer(syncparam)

	proc.sync = blocksync

	fac := types.NewMessageFactory(
		types.NewGenesisFactory(proc.rosterFac),
		blockFac,
		param.Mino.GetAddressFactory(),
		param.Cosi.GetSignatureFactory(),
		csFac,
	)

	proc.MessageFactory = fac

	actor, err := param.Cosi.Listen(proc)
	if err != nil {
		return nil, xerrors.Errorf("creating cosi failed: %v", err)
	}

	s := &Service{
		processor:                proc,
		me:                       param.Mino.GetAddress(),
		rpc:                      mino.MustCreateRPC(param.Mino, rpcName, proc, fac),
		actor:                    actor,
		val:                      param.Validation,
		verifierFac:              param.Cosi.GetVerifierFactory(),
		timeoutRound:             RoundTimeout,
		timeoutRoundAfterFailure: RoundTimeout,
		timeoutViewchange:        RoundTimeout,
		events:                   make(chan ordering.Event, 1),
		closing:                  make(chan struct{}),
		closed:                   make(chan struct{}),
	}

	// Pool will filter the transaction that are already accepted by this
	// service.
	param.Pool.AddFilter(poolFilter{tree: proc.tree, srvc: param.Validation})

	go s.main()

	go s.watchBlocks()

	if s.genesis.Exists() {
		// If the genesis already exists, the service can start right away to
		// participate in the chain.
		close(s.started)
	}

	return s, nil
}

// Setup creates a genesis block and sends it to the collective authority.
func (s *Service) Setup(ctx context.Context, ca crypto.CollectiveAuthority) error {
	err := s.storeGenesis(authority.FromAuthority(ca), nil)
	if err != nil {
		return xerrors.Errorf("creating genesis: %v", err)
	}

	genesis, err := s.genesis.Get()
	if err != nil {
		return xerrors.Errorf("failed to read genesis: %v", err)
	}

	resps, err := s.rpc.Call(ctx, types.NewGenesisMessage(genesis), ca)
	if err != nil {
		return xerrors.Errorf("sending genesis: %v", err)
	}

	for resp := range resps {
		_, err := resp.GetMessageOrError()
		if err != nil {
			return xerrors.Errorf("one request failed: %v", err)
		}
	}

	s.logger.Info().
		Int("roster", ca.Len()).
		Stringer("digest", genesis.GetHash()).
		Msg("new chain has been created")

	return nil
}

// GetProof implements ordering.Service. It returns the proof of absence or
// inclusion for the latest block. The proof integrity is not verified as this
// is assumed the node is acting correctly so the data is anyway consistent. The
// proof must be verified by the caller when leaving the trusted environment,
// for instance when the proof is sent over the network.
func (s *Service) GetProof(key []byte) (ordering.Proof, error) {
	tree, unlock := s.tree.GetWithLock()
	defer unlock()

	path, err := tree.GetPath(key)
	if err != nil {
		return nil, xerrors.Errorf("reading path: %v", err)
	}

	// The chain is fetched while having the lock of the tree cache so that
	// there is no race between the two stores when finalizing a block.
	chain, err := s.blocks.GetChain()
	if err != nil {
		return nil, xerrors.Errorf("reading chain: %v", err)
	}

	return newProof(path, chain), nil
}

// GetStore implements ordering.Service. It returns the current tree as a
// read-only storage.
func (s *Service) GetStore() store.Readable {
	return s.tree.Get()
}

// GetRoster returns the current roster of the service.
func (s *Service) GetRoster() (authority.Authority, error) {
	return s.getCurrentRoster()
}

// Watch implements ordering.Service. It returns a channel that will be
// populated with new incoming blocks and some information about them. The
// channel must be listened at all time and the context must be closed when
// done.
func (s *Service) Watch(ctx context.Context) <-chan ordering.Event {
	obs := observer{ch: make(chan ordering.Event, 1)}

	s.watcher.Add(obs)

	go func() {
		<-ctx.Done()
		s.watcher.Remove(obs)
		close(obs.ch)
	}()

	return obs.ch
}

// Close implements ordering.Service. It gracefully closes the service. It will
// announce the closing request and wait for the current to end before
// returning.
func (s *Service) Close() error {
	close(s.closing)
	<-s.closed

	return nil
}

func (s *Service) watchBlocks() {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-s.closing
		cancel()
	}()

	linkCh := s.blocks.Watch(ctx)

	for link := range linkCh {
		// 1. Remove the transactions from the pool to avoid duplicates.
		for _, res := range link.GetBlock().GetData().GetTransactionResults() {
			s.pool.Remove(res.GetTransaction())
		}

		// 2. Update the current membership.
		err := s.refreshRoster()
		if err != nil {
			s.logger.Err(err).Msg("roster refresh failed")
		}

		event := ordering.Event{
			Index:        link.GetBlock().GetIndex(),
			Transactions: link.GetBlock().GetData().GetTransactionResults(),
		}

		// 3. Notify the main loop that a new block has been created, but ignore
		// if the channel is busy.
		select {
		case s.events <- event:
		default:
		}

		// 4. Notify the new block to potential listeners.
		s.watcher.Notify(event)

		s.logger.Info().
			Uint64("index", link.GetBlock().GetIndex()).
			Stringer("root", link.GetBlock().GetTreeRoot()).
			Msg("block event")
	}
}

func (s *Service) refreshRoster() error {
	roster, err := s.getCurrentRoster()
	if err != nil {
		return xerrors.Errorf("reading roster: %v", err)
	}

	err = s.pool.SetPlayers(roster)
	if err != nil {
		return xerrors.Errorf("updating tx pool: %v", err)
	}

	return nil
}

func (s *Service) main() error {
	defer close(s.closed)

	select {
	case <-s.started:
		// A genesis block has been set, the node will then follow the chain
		// related to it.
		s.logger.Info().Msg("node has started following the chain")
	case <-s.closing:
		return nil
	}

	// Update the components that need to learn about the participants like the
	// transaction pool.
	err := s.refreshRoster()
	if err != nil {
		return xerrors.Errorf("refreshing roster: %v", err)
	}

	s.logger.Debug().Msg("node has started")

	backoff := float64(0)

	for {
		// When a round failure occurs, it sleeps with a given backoff to give a
		// chance to the system to recover without exhausting the resources.
		time.Sleep(calculateBackoff(backoff))

		select {
		case <-s.closing:
			return nil
		default:
			ctx, cancel := context.WithCancel(context.Background())

			go func() {
				select {
				case <-s.closing:
					cancel()
				case <-ctx.Done():
				}
			}()

			err := s.doRound(ctx)
			cancel()

			if err != nil {
				if calculateBackoff(backoff+1) < RoundMaxWait {
					backoff++
				}

				s.logger.Err(err).Msg("round failed")
			} else {
				backoff = 0
			}
		}
	}
}

func (s *Service) doRound(ctx context.Context) error {
	roster, err := s.getCurrentRoster()
	if err != nil {
		return xerrors.Errorf("reading roster: %v", err)
	}

	leader, err := s.pbftsm.GetLeader()
	if err != nil {
		return xerrors.Errorf("reading leader: %v", err)
	}

	timeout := s.timeoutRound
	if s.failedRound {
		timeout = s.timeoutRoundAfterFailure
	}

	for !s.me.Equal(leader) {
		// Only enters the loop if the node is not the leader. It has to wait
		// for the new block, or the round timeout, to proceed.

		select {
		case <-time.After(timeout):
			if s.pool.Len() == 0 {
				// When the pool of transactions is empty, the round is aborted
				// and everything restart.
				return nil
			}

			s.logger.Warn().Msg("round reached the timeout")

			// Mark that the view change happened during this round.
			s.failedRound = true

			ctx, cancel := context.WithTimeout(ctx, s.timeoutViewchange)

			view, err := s.pbftsm.Expire(s.me)
			if err != nil {
				cancel()
				return xerrors.Errorf("pbft expire failed: %v", err)
			}

			viewMsg := types.NewViewMessage(view.GetID(), view.GetLeader(), view.GetSignature())

			resps, err := s.rpc.Call(ctx, viewMsg, roster)
			if err != nil {
				cancel()
				return xerrors.Errorf("rpc failed to send views: %v", err)
			}

			for resp := range resps {
				_, err = resp.GetMessageOrError()
				if err != nil {
					s.logger.Warn().Err(err).Str("to", resp.GetFrom().String()).Msg("view propagation failure")
				}
			}

			statesCh := s.pbftsm.Watch(ctx)

			state := s.pbftsm.GetState()
			var more bool

			for state == pbft.ViewChangeState {
				state, more = <-statesCh
				if !more {
					cancel()
					return xerrors.New("viewchange failed")
				}
			}

			s.logger.Debug().Msgf("view change successful for %d", viewMsg.GetLeader())

			cancel()
			return nil
		case <-s.events:
			// As a child, a block has been committed thus the previous view
			// change succeeded.
			s.failedRound = false

			// A block has been created meaning that the round is over.
			return nil
		case <-s.closing:
			return nil
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	s.logger.Debug().Uint64("index", s.blocks.Len()).Msg("round has started")

	// Send a synchronization to the roster so that they can learn about the
	// latest block of the chain.
	err = s.sync.Sync(ctx, roster, blocksync.Config{MinHard: threshold.ByzantineThreshold(roster.Len())})
	if err != nil {
		return xerrors.Errorf("sync failed: %v", err)
	}

	s.logger.Debug().Uint64("index", s.blocks.Len()).Msg("pbft has started")

	err = s.doPBFT(ctx)
	if err != nil {
		return xerrors.Errorf("pbft failed: %v", err)
	}

	// The leader can be a new leader coming from a view change, so it resets
	// the value as a round has finished.
	s.failedRound = false

	return nil
}

func (s *Service) doPBFT(ctx context.Context) error {
	var id types.Digest
	var block types.Block

	if s.pbftsm.GetState() >= pbft.CommitState {
		// The node is already committed to a block, which means enough nodes
		// have accepted, but somehow the finalization failed.
		id, block = s.pbftsm.GetCommit()
	} else {
		txs := s.pool.Gather(ctx, pool.Config{Min: 1})
		if len(txs) == 0 {
			s.logger.Debug().Msg("no transaction in pool")

			return nil
		}

		s.logger.Debug().
			Int("num", len(txs)).
			Msg("transactions have been found")

		if ctx.Err() != nil {
			// Don't bother trying PBFT if the context is done.
			return ctx.Err()
		}

		data, root, err := s.prepareData(txs)
		if err != nil {
			return xerrors.Errorf("failed to prepare data: %v", err)
		}

		block, err = types.NewBlock(
			data,
			types.WithTreeRoot(root),
			types.WithIndex(uint64(s.blocks.Len())),
			types.WithHashFactory(s.hashFactory))

		if err != nil {
			return xerrors.Errorf("creating block failed: %v", err)
		}

		id, err = s.pbftsm.Prepare(s.me, block)
		if err != nil {
			return xerrors.Errorf("pbft prepare failed: %v", err)
		}
	}

	roster, err := s.getCurrentRoster()
	if err != nil {
		return xerrors.Errorf("read roster failed: %v", err)
	}

	// 1. Prepare phase
	req := types.NewBlockMessage(block, s.prepareViews())

	sig, err := s.actor.Sign(ctx, req, roster)
	if err != nil {
		return xerrors.Errorf("prepare signature failed: %v", err)
	}

	s.logger.Debug().Str("signature", fmt.Sprintf("%v", sig)).Msg("prepare done")

	// 2. Commit phase
	commit := types.NewCommit(id, sig)

	sig, err = s.actor.Sign(ctx, commit, roster)
	if err != nil {
		return xerrors.Errorf("commit signature failed: %v", err)
	}

	s.logger.Debug().Str("signature", fmt.Sprintf("%v", sig)).Msg("commit done")

	// 3. Propagation phase
	done := types.NewDone(id, sig)

	resps, err := s.rpc.Call(ctx, done, roster)
	if err != nil {
		return xerrors.Errorf("propagation failed: %v", err)
	}

	for resp := range resps {
		_, err = resp.GetMessageOrError()
		if err != nil {
			s.logger.Warn().Err(err).Msg("propagation failed")
		}
	}

	// 4. Wake up new participants so that they can learn about the chain.
	err = s.wakeUp(ctx, roster)
	if err != nil {
		return xerrors.Errorf("wake up failed: %v", err)
	}

	return nil
}

func (s *Service) prepareViews() map[mino.Address]types.ViewMessage {
	views := s.pbftsm.GetViews()
	msgs := make(map[mino.Address]types.ViewMessage)

	for addr, view := range views {
		msgs[addr] = types.NewViewMessage(view.GetID(), view.GetLeader(), view.GetSignature())
	}

	return msgs
}

func (s *Service) prepareData(txs []txn.Transaction) (data validation.Result, id types.Digest, err error) {
	var stageTree hashtree.StagingTree

	stageTree, err = s.tree.Get().Stage(func(snap store.Snapshot) error {
		data, err = s.val.Validate(snap, txs)
		if err != nil {
			return xerrors.Errorf("validation failed: %v", err)
		}

		return nil
	})

	if err != nil {
		err = xerrors.Errorf("staging tree failed: %v", err)
		return
	}

	copy(id[:], stageTree.GetRoot())

	return
}

func (s *Service) wakeUp(ctx context.Context, ro authority.Authority) error {
	newRoster, err := s.getCurrentRoster()
	if err != nil {
		return xerrors.Errorf("read roster failed: %v", err)
	}

	changeset := ro.Diff(newRoster)

	genesis, err := s.genesis.Get()
	if err != nil {
		return xerrors.Errorf("read genesis failed: %v", err)
	}

	resps, err := s.rpc.Call(ctx, types.NewGenesisMessage(genesis), mino.NewAddresses(changeset.GetNewAddresses()...))
	if err != nil {
		return xerrors.Errorf("rpc failed: %v", err)
	}

	for resp := range resps {
		_, err := resp.GetMessageOrError()
		if err != nil {
			s.logger.Warn().Err(err).Msg("wake up failed")
		}
	}

	return nil
}

type observer struct {
	ch chan ordering.Event
}

func (obs observer) NotifyCallback(event interface{}) {
	obs.ch <- event.(ordering.Event)
}

func calculateBackoff(backoff float64) time.Duration {
	return time.Duration(math.Pow(2, backoff)) * RoundWait
}

// PoolFilter is a filter to drop transactions which are already included in the
// block or simply with an invalid nonce.
//
// - implements pool.Filter
type poolFilter struct {
	tree blockstore.TreeCache
	srvc validation.Service
}

// Accept implements pool.Filter. It returns an error if the transaction exists
// already or the nonce is invalid.
func (f poolFilter) Accept(tx txn.Transaction, leeway validation.Leeway) error {
	store := f.tree.Get()

	err := f.srvc.Accept(store, tx, leeway)
	if err != nil {
		return xerrors.Errorf("unacceptable transaction: %v", err)
	}

	return nil
}
