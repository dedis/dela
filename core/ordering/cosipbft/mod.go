package cosipbft

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/blocksync"
	"go.dedis.ch/dela/core/ordering/cosipbft/pbft"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/tap/pool"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

const (
	// RoundTimeout is the maximum of time the service waits for an event to
	// happen.
	RoundTimeout = 10 * time.Second
)

// Service is an ordering service using collective signatures combined with PBFT
// to create a chain of blocks.
type Service struct {
	*processor

	me    mino.Address
	rpc   mino.RPC
	actor cosi.Actor
	val   validation.Service

	logger  zerolog.Logger
	timeout time.Duration
	closing chan struct{}
}

type serviceTemplate struct {
	hashFac crypto.HashFactory
	blocks  blockstore.BlockStore
	genesis blockstore.GenesisStore
}

// ServiceOption is the type of option to set some fields of the service.
type ServiceOption func(*serviceTemplate)

// ServiceParam is the different components to provide to the service. All the
// fields are mandatory and it will panic if any is nil.
type ServiceParam struct {
	Mino       mino.Mino
	Cosi       cosi.CollectiveSigning
	Validation validation.Service
	Pool       pool.Pool
	Tree       hashtree.Tree
}

// NewService starts a new service.
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
	proc.rosterFac = roster.NewFactory(param.Mino.GetAddressFactory(), param.Cosi.GetPublicKeyFactory())
	proc.tree = blockstore.NewTreeCache(param.Tree)

	pcparam := pbft.StateMachineParam{
		Validation:      param.Validation,
		VerifierFactory: param.Cosi.GetVerifierFactory(),
		Blocks:          tmpl.blocks,
		Genesis:         tmpl.genesis,
		Tree:            proc.tree,
	}

	proc.pbftsm = pbft.NewStateMachine(pcparam)

	blockFac := types.NewBlockFactory(param.Validation.GetFactory())
	linkFac := types.NewBlockLinkFactory(blockFac, param.Cosi.GetSignatureFactory())

	syncparam := blocksync.SyncParam{
		Mino:        param.Mino,
		Blocks:      tmpl.blocks,
		PBFT:        proc.pbftsm,
		LinkFactory: linkFac,
	}

	blocksync, err := blocksync.NewSynchronizer(syncparam)
	if err != nil {
		return nil, err
	}

	proc.sync = blocksync

	fac := types.NewMessageFactory(
		types.NewGenesisFactory(proc.rosterFac),
		blockFac,
		param.Cosi.GetSignatureFactory(),
	)

	proc.MessageFactory = fac

	rpc, err := param.Mino.MakeRPC("cosipbft", proc, fac)
	if err != nil {
		return nil, xerrors.Errorf("rpc failed: %v", err)
	}

	actor, err := param.Cosi.Listen(proc)
	if err != nil {
		return nil, xerrors.Errorf("cosi failed: %v", err)
	}

	s := &Service{
		processor: proc,
		me:        param.Mino.GetAddress(),
		rpc:       rpc,
		actor:     actor,
		val:       param.Validation,
		logger:    dela.Logger.With().Str("addr", param.Mino.GetAddress().String()).Logger(),
		timeout:   RoundTimeout,
		closing:   make(chan struct{}),
	}

	go func() { s.run(s.main(), "main loop failed") }()
	go s.watchBlocks()

	return s, nil
}

// Setup creates a genesis block and sends it to the collective authority.
func (s *Service) Setup(ca crypto.CollectiveAuthority) error {
	genesis, err := types.NewGenesis(ca)
	if err != nil {
		return err
	}

	ctx := context.Background()

	resps, err := s.rpc.Call(ctx, types.NewGenesisMessage(genesis), ca)
	if err != nil {
		return xerrors.Errorf("call failed: %v", err)
	}

	for resp := range resps {
		_, err := resp.GetMessageOrError()
		if err != nil {
			return xerrors.Errorf("propagation failed: %v", err)
		}
	}

	return nil
}

// GetProof implements ordering.Service.
func (s *Service) GetProof(key []byte) (ordering.Proof, error) {
	return nil, nil
}

// Watch implements ordering.Service.
func (s *Service) Watch(ctx context.Context) <-chan ordering.Event {
	ch := make(chan ordering.Event, 1)

	obs := observer{ch: ch}
	s.watcher.Add(obs)

	go func() {
		<-ctx.Done()
		s.watcher.Remove(obs)
	}()

	return ch
}

// Close closes.
func (s *Service) Close() error {
	close(s.closing)
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
		for _, res := range link.GetTo().GetData().GetTransactionResults() {
			s.pool.Remove(res.GetTransaction())
		}

		// 2. Notify the new block to potential listeners.
		s.watcher.Notify(ordering.Event{
			Index: link.GetTo().GetIndex(),
		})
	}
}

func (s *Service) main() error {
	// Try to init, or wait for a genesis block from a setup or a leader.
	s.init()

	s.logger.Debug().Msg("node has started")

	for {
		select {
		case <-s.closing:
			return nil
		default:
			err := s.doRound()
			if err != nil {
				return xerrors.Errorf("round failed: %v", err)
			}
		}
	}
}

func (s *Service) init() error {
	<-s.started

	// Look up the latest roster of the chain and update the pool to send rumors
	// to it.
	roster, err := s.getCurrentRoster()
	if err != nil {
		return err
	}

	s.pool.SetPlayers(roster)

	return nil
}

func (s *Service) doRound() error {
	roster, err := s.getCurrentRoster()
	if err != nil {
		return xerrors.Errorf("read roster failed: %v", err)
	}

	if s.pbftsm.GetState() == pbft.InitialState {
		err = s.pbftsm.PrePrepare(roster)
		if err != nil {
			return xerrors.Errorf("pbft pre-prepare failed: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := s.Watch(ctx)

	for !s.me.Equal(s.pbftsm.GetLeader()) {
		// Only enters the loop if the node is not the leader. It has to wait
		// for the new block, or the round timeout, to proceed.

		select {
		case <-time.After(s.timeout):
			s.logger.Warn().Msg("round reached the timeout")

			statesCh := s.pbftsm.Watch(ctx)

			view, err := s.pbftsm.Expire(s.me)
			if err != nil {
				return xerrors.Errorf("pbft expire failed: %v", err)
			}

			resps, err := s.rpc.Call(ctx, types.NewViewMessage(view.ID, view.Leader), roster)
			if err != nil {
				return xerrors.Errorf("rpc failed: %v", err)
			}

			for resp := range resps {
				_, err = resp.GetMessageOrError()
				if err != nil {
					s.logger.Warn().Err(err).Msg("view propagation failure")
				}
			}

			for state := range statesCh {
				if state == pbft.PrePrepareState {
					break
				}
			}

			s.logger.Debug().Msg("view change successful")
		case <-events:
			// A block has been created meaning that the round is over.
			return nil
		case <-s.closing:
			return nil
		}
	}

	s.logger.Debug().Msg("round has started")

	// Send a synchronization to the roster so that they can learn about the
	// latest block of the chain.
	err = s.waitSync(ctx, roster)
	if err != nil {
		return err
	}

	// TODO: check that no committed block exists in the case of a leader
	// failure when propagating the collective signature.
	txs := s.collectTxs(ctx)

	block, err := s.prepareBlock(txs)
	if err != nil {
		return err
	}

	err = s.doPBFT(ctx, block)
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) waitSync(ctx context.Context, roster viewchange.Authority) error {
	syncEvents, err := s.sync.Sync(ctx, roster)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt := <-syncEvents:
			// TODO: get threshold
			if evt.Hard >= roster.Len() {
				return nil
			}
		}
	}
}

func (s *Service) collectTxs(ctx context.Context) []tap.Transaction {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	events := s.pool.Watch(ctx)

	if s.pool.Len() > 0 {
		return s.pool.GetAll()
	}

	for evt := range events {
		if evt.Len > 0 {
			return s.pool.GetAll()
		}
	}

	return nil
}

func (s *Service) prepareBlock(txs []tap.Transaction) (*types.Block, error) {
	var data validation.Data

	stageTree, err := s.tree.Get().Stage(func(snap store.Snapshot) error {
		var err error
		data, err = s.val.Validate(snap, txs)
		if err != nil {
			return xerrors.Errorf("validation failed: %v", err)
		}

		return nil
	})

	if err != nil {
		return nil, xerrors.Errorf("staging tree failed: %v", err)
	}

	root := types.Digest{}
	copy(root[:], stageTree.GetRoot())

	index := s.blocks.Len() + 1

	block, err := types.NewBlock(
		data,
		types.WithTreeRoot(root),
		types.WithIndex(uint64(index)),
		types.WithHashFactory(s.hashFactory),
	)

	if err != nil {
		return nil, xerrors.Errorf("creating block failed: %v", err)
	}

	return &block, nil
}

func (s *Service) doPBFT(ctx context.Context, block *types.Block) error {
	roster, err := s.getCurrentRoster()
	if err != nil {
		return xerrors.Errorf("read roster failed: %v", err)
	}

	id, err := s.pbftsm.Prepare(*block)
	if err != nil {
		return xerrors.Errorf("pbft prepare failed: %v", err)
	}

	// 1. Prepare phase
	req := types.NewBlockMessage(*block)

	sig, err := s.actor.Sign(ctx, req, roster)
	if err != nil {
		return xerrors.Errorf("prepare phase failed: %v", err)
	}

	s.logger.Debug().Str("signature", fmt.Sprintf("%v", sig)).Msg("prepare done")

	// 2. Commit phase
	commit := types.NewCommit(id, sig)

	sig, err = s.actor.Sign(ctx, commit, roster)
	if err != nil {
		return xerrors.Errorf("commit phase failed: %v", err)
	}

	s.logger.Debug().Str("signature", fmt.Sprintf("%v", sig)).Msg("commit done")

	// 3. Propagation phase
	done := types.NewDone(id, sig)

	resps, err := s.rpc.Call(ctx, done, roster)
	if err != nil {
		return xerrors.Errorf("rpc failed: %v", err)
	}

	for resp := range resps {
		_, err = resp.GetMessageOrError()
		if err != nil {
			s.logger.Warn().Err(err).Msg("propagation failed")
		}
	}

	return nil
}

func (s *Service) run(err error, msg string) {
	if err != nil {
		s.logger.Warn().Err(err).Msg(msg)
	}
}

type observer struct {
	ch chan ordering.Event
}

func (obs observer) NotifyCallback(event interface{}) {
	obs.ch <- event.(ordering.Event)
}
