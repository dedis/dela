package cosipbft

import (
	"context"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
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
	RoundTimeout = 500 * time.Millisecond
)

// Service is an ordering service using collective signatures combined with PBFT
// to create a chain of blocks.
type Service struct {
	*handler

	me    mino.Address
	rpc   mino.RPC
	pool  pool.Pool
	actor cosi.Actor

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
	rosterFac := roster.NewFactory(m.GetAddressFactory(), c.GetPublicKeyFactory())

	h := newHandler(tree, val, rosterFac)

	fac := types.NewMessageFactory(
		types.NewGenesisFactory(rosterFac),
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
		pool:    p,
		actor:   actor,
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
	genesis := types.NewGenesis(ca)

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

	txs := s.pool.GetAll()

	var data validation.Data

	stageTree, err := s.tree.Stage(func(snap store.Snapshot) error {
		var err error
		data, err = s.validation.Validate(snap, txs)
		return err
	})

	if err != nil {
		return err
	}

	block := types.NewBlock(data, stageTree.GetRoot())

	err = s.doPBFT(block)
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) doPBFT(block types.Block) error {
	roster := s.genesis.GetRoster()

	ctx := context.Background()

	// 1. Prepare phase
	req := types.BlockMessage{}

	sig, err := s.actor.Sign(ctx, req, roster)
	if err != nil {
		return xerrors.Errorf("prepare phase failed: %v", err)
	}

	dela.Logger.Info().Msg("signature done")

	// 2. Commit phase

	commit := types.NewCommit(sig)

	sig, err = s.actor.Sign(ctx, commit, roster)
	if err != nil {
		return err
	}

	dela.Logger.Info().Msg("pbft done")

	// 3. Propagation phase

	done := types.NewDone(sig)

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
