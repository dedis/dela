package cosipbft

import (
	"context"
	"fmt"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
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
	RoundTimeout = 100 * time.Millisecond
)

// Service is an ordering service using collective signatures combined with PBFT
// to create a chain of blocks.
type Service struct {
	*processor

	me    mino.Address
	rpc   mino.RPC
	actor cosi.Actor
	val   validation.Service
	pbft  pbft.StateMachine

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
	proc.tree = param.Tree

	pcparam := pbft.StateMachineParam{
		Validation:      param.Validation,
		VerifierFactory: param.Cosi.GetVerifierFactory(),
		Blocks:          tmpl.blocks,
		Genesis:         tmpl.genesis,
	}

	proc.committer = pbft.NewStateMachine(pcparam)

	fac := types.NewMessageFactory(
		types.NewGenesisFactory(proc.rosterFac),
		types.NewBlockFactory(param.Validation.GetFactory()),
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
		pbft:      proc.committer,
		closing:   make(chan struct{}),
	}

	go func() {
		err := s.main()
		if err != nil {
			dela.Logger.Err(err).Msg("main loop stopped")
		}
	}()

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

func (s *Service) main() error {
	// Try to init, or wait for a genesis block from a setup or a leader.
	s.init()

	dela.Logger.Info().Msg("node has started")

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
		return err
	}

	if s.pbft.GetState() == pbft.InitialState {
		err = s.pbft.PrePrepare(roster)
		if err != nil {
			return err
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := s.Watch(ctx)

	for !s.me.Equal(s.pbft.GetLeader()) {
		// Only enters the loop if the node is not the leader. It has to wait
		// for the new block, or the round timeout, to proceed.

		select {
		case <-time.After(RoundTimeout):
			dela.Logger.Warn().Str("addr", fmt.Sprintf("%v", s.me)).Msg("round reached the timeout")

			statesCh := s.pbft.Watch(ctx)

			view, err := s.pbft.Expire(s.me)
			if err != nil {
				return err
			}

			resps, err := s.rpc.Call(ctx, types.NewViewMessage(view.ID, view.Leader), roster)
			if err != nil {
				return err
			}

			for resp := range resps {
				_, err = resp.GetMessageOrError()
				if err != nil {
					dela.Logger.Warn().Err(err).Msg("view propagation failure")
				}
			}

			for state := range statesCh {
				if state == pbft.PrePrepareState {
					break
				}
			}

			dela.Logger.Info().Str("addr", fmt.Sprintf("%v", s.me)).Msg("view change successful")
		case <-events:
			// A block has been created meaning that the round is over.
			return nil
		case <-s.closing:
			return nil
		}
	}

	dela.Logger.Info().Str("addr", fmt.Sprintf("%v", s.me)).Msg("round has started")

	// TODO: check that no committed block exists in the case of a leader failure
	// when propagating the collective signature.
	txs := s.collectTxs(ctx)

	var data validation.Data

	stageTree, err := s.tree.Stage(func(snap store.Snapshot) error {
		var err error
		data, err = s.val.Validate(snap, txs)
		return err
	})

	if err != nil {
		return err
	}

	root := types.Digest{}
	copy(root[:], stageTree.GetRoot())

	block, err := types.NewBlock(data, types.WithTreeRoot(root))
	if err != nil {
		return err
	}

	err = s.doPBFT(ctx, block)
	if err != nil {
		return err
	}

	return nil
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

func (s *Service) doPBFT(ctx context.Context, block types.Block) error {
	roster, err := s.getCurrentRoster()
	if err != nil {
		return err
	}

	id, err := s.committer.Prepare(block, s.tree)
	if err != nil {
		return err
	}

	// 1. Prepare phase
	req := types.NewBlockMessage(block)

	sig, err := s.actor.Sign(ctx, req, roster)
	if err != nil {
		return xerrors.Errorf("prepare phase failed: %v", err)
	}

	dela.Logger.Info().Str("signature", fmt.Sprintf("%v", sig)).Msg("prepare done")

	// 2. Commit phase
	commit := types.NewCommit(id, sig)

	sig, err = s.actor.Sign(ctx, commit, roster)
	if err != nil {
		return err
	}

	dela.Logger.Info().Str("signature", fmt.Sprintf("%v", sig)).Msg("commit done")

	// 3. Propagation phase
	done := types.NewDone(id, sig)

	resps, err := s.rpc.Call(ctx, done, roster)
	if err != nil {
		return err
	}

	for resp := range resps {
		_, err = resp.GetMessageOrError()
		if err != nil {
			return err
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
