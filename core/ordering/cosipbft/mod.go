package cosipbft

import (
	"context"
	"fmt"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
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
	RoundTimeout = 200 * time.Millisecond
)

// Service is an ordering service using collective signatures combined with PBFT
// to create a chain of blocks.
type Service struct {
	*handler

	me    mino.Address
	rpc   mino.RPC
	actor cosi.Actor
	val   validation.Service

	closing chan struct{}
}

// NewService starts a new service.
func NewService(
	m mino.Mino,
	c cosi.CollectiveSigning,
	p pool.Pool,
	tree hashtree.Tree,
	val validation.Service,
) (*Service, error) {
	hashFactory := crypto.NewSha256Factory()
	rosterFac := roster.NewFactory(m.GetAddressFactory(), c.GetPublicKeyFactory())
	genesis := blockstore.NewGenesisStore()
	blocks := blockstore.NewInMemory()
	committer := newTwoPhaseCommit(val, c, hashFactory, genesis, blocks)

	h := newHandler(tree, rosterFac, committer, hashFactory, genesis, blocks, p)

	fac := types.NewMessageFactory(
		types.NewGenesisFactory(rosterFac),
		types.NewBlockFactory(val.GetFactory()),
		c.GetSignatureFactory(),
	)

	h.MessageFactory = fac

	rpc, err := m.MakeRPC("cosipbft", h, fac)
	if err != nil {
		return nil, xerrors.Errorf("rpc failed: %v", err)
	}

	actor, err := c.Listen(h)
	if err != nil {
		return nil, xerrors.Errorf("cosi failed: %v", err)
	}

	s := &Service{
		handler: h,
		me:      m.GetAddress(),
		rpc:     rpc,
		actor:   actor,
		val:     val,
		closing: make(chan struct{}),
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
		roundTimeout := time.After(RoundTimeout)

		select {
		case <-s.closing:
			return nil
		case <-roundTimeout:
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

	_, idx := roster.GetPublicKey(s.me)
	if idx != 0 {
		// Only the leader tries to create a block.
		return nil
	}

	dela.Logger.Info().Msg("round has started")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	block, err := types.NewBlock(data, types.WithTreeRoot(stageTree.GetRoot()))
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

	id, err := s.committer.Propose(block, s.tree)
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
