package blocksync

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/blocksync/types"
	"go.dedis.ch/dela/core/ordering/cosipbft/pbft"
	otypes "go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

// DefaultSync is a block synchronizer that allow soft and hard synchronization
// of the participants. A soft threshold means that a given number of
// participants have updated the latest index, whereas a hard one means that
// they have stored all the blocks up to the latest index.
//
// - implements blocksync.Synchronizer
type defaultSync struct {
	logger zerolog.Logger
	rpc    mino.RPC
	pbftsm pbft.StateMachine
	blocks blockstore.BlockStore
	latest *uint64
}

// SyncParam is the parameter object to create a new synchronizer.
type SyncParam struct {
	Mino            mino.Mino
	PBFT            pbft.StateMachine
	Blocks          blockstore.BlockStore
	Genesis         blockstore.GenesisStore
	LinkFactory     otypes.LinkFactory
	ChainFactory    otypes.ChainFactory
	VerifierFactory crypto.VerifierFactory
}

// NewSynchronizer creates a new block synchronizer.
func NewSynchronizer(param SyncParam) (Synchronizer, error) {
	latest := param.Blocks.Len()

	logger := dela.Logger.With().Str("addr", param.Mino.GetAddress().String()).Logger()

	h := &handler{
		logger:      logger,
		latest:      &latest,
		genesis:     param.Genesis,
		blocks:      param.Blocks,
		pbftsm:      param.PBFT,
		verifierFac: param.VerifierFactory,
	}

	fac := types.NewMessageFactory(param.LinkFactory, param.ChainFactory)

	rpc, err := param.Mino.MakeRPC("blocksync", h, fac)
	if err != nil {
		return nil, xerrors.Errorf("rpc creation failed: %v", err)
	}

	s := defaultSync{
		logger: logger,
		rpc:    rpc,
		pbftsm: param.PBFT,
		blocks: param.Blocks,
		latest: &latest,
	}

	return s, nil
}

// GetLatest implements blocksync.Synchronizer. It returns the latest index
// known by the instance.
func (s defaultSync) GetLatest() uint64 {
	return atomic.LoadUint64(s.latest)
}

// Sync implements blocksync.Synchronizer. it starts a routine to first
// soft-sync the participants and then send the blocks when necessary.
func (s defaultSync) Sync(ctx context.Context, players mino.Players, cfg Config) error {
	if s.blocks.Len() == 0 {
		// When the store is empty, that means that the participants are all
		// synchronized anyway as there is no block.
		return nil
	}

	sender, rcvr, err := s.rpc.Stream(ctx, players)
	if err != nil {
		return xerrors.Errorf("stream failed: %v", err)
	}

	// 1. Send the announcement message to everyone so that they can learn about
	// the latest block.
	chain, err := s.blocks.GetChain()
	if err != nil {
		return xerrors.Errorf("failed to read chain: %v", err)
	}

	errs := sender.Send(types.NewSyncMessage(chain), iter2arr(players.AddressIterator())...)
	for err := range errs {
		if err != nil {
			return xerrors.Errorf("announcement failed: %v", err)
		}
	}

	// 2. Wait for the hard synchronization to end. It can be interrupted with
	// the context.
	soft := map[mino.Address]struct{}{}
	hard := map[mino.Address]struct{}{}

	for len(soft) < cfg.MinSoft || len(hard) < cfg.MinHard {
		from, msg, err := rcvr.Recv(ctx)
		if err != nil {
			return xerrors.Errorf("receiver failed: %v", err)
		}

		switch in := msg.(type) {
		case types.SyncRequest:
			_, found := soft[from]
			if found {
				s.logger.Warn().Msg("found duplicate request")
				continue
			}

			soft[from] = struct{}{}

			err := s.syncNode(in.GetFrom(), sender, from)
			if err != nil {
				return xerrors.Errorf("synchronizing node %v: %v", from, err)
			}
		case types.SyncAck:
			soft[from] = struct{}{}
			hard[from] = struct{}{}
		}
	}

	return nil
}

func (s defaultSync) syncNode(from uint64, sender mino.Sender, to mino.Address) error {
	for i := from; i < s.blocks.Len(); i++ {
		s.logger.Debug().Uint64("index", i).Str("to", to.String()).Msg("send block")

		link, err := s.blocks.GetByIndex(i)
		if err != nil {
			return xerrors.Errorf("couldn't get block: %v", err)
		}

		err = <-sender.Send(types.NewSyncReply(link), to)
		if err != nil {
			return xerrors.Errorf("failed to send block: %v", err)
		}
	}

	return nil
}

type handler struct {
	sync.Mutex
	mino.UnsupportedHandler

	latest      *uint64
	logger      zerolog.Logger
	blocks      blockstore.BlockStore
	genesis     blockstore.GenesisStore
	pbftsm      pbft.StateMachine
	verifierFac crypto.VerifierFactory
}

func (h *handler) Stream(out mino.Sender, in mino.Receiver) error {
	ctx := context.Background()

	m, orch, err := h.waitAnnounce(ctx, in)
	if err != nil {
		return xerrors.Errorf("no announcement: %v", err)
	}

	h.logger.Debug().
		Uint64("index", m.GetLatestIndex()).
		Msg("received synchronization message")

	genesis, err := h.genesis.Get()
	if err != nil {
		return xerrors.Errorf("reading genesis: %v", err)
	}

	err = m.GetChain().Verify(genesis, h.verifierFac)
	if err != nil {
		return xerrors.Errorf("failed to verify chain: %v", err)
	}

	if m.GetLatestIndex() <= h.blocks.Len() {
		// The block storage has already all the block known so far so we can
		// send the hard-sync acknowledgement.
		return h.ack(out, orch)
	}

	// At this point, the synchronization can only happen on one thread, so it
	// waits for the lock to be free, which means that in the meantime some
	// blocks might have been stored but the request is sent with the most
	// up-to-date block index, so it won't catch up twice the same block.
	h.Lock()
	defer h.Unlock()

	// Update the latest index through atomic operations as it can be read
	// asynchronously from the getter.
	if m.GetLatestIndex() > atomic.LoadUint64(h.latest) {
		atomic.StoreUint64(h.latest, m.GetLatestIndex())
	}

	err = <-out.Send(types.NewSyncRequest(h.blocks.Len()), orch)
	if err != nil {
		return xerrors.Errorf("sending request failed: %v", err)
	}

	for h.blocks.Len() < m.GetLatestIndex() {
		_, msg, err := in.Recv(ctx)
		if err != nil {
			return xerrors.Errorf("receiver failed: %v", err)
		}

		reply, ok := msg.(types.SyncReply)
		if ok {
			h.logger.Debug().
				Uint64("index", reply.GetLink().GetBlock().GetIndex()).
				Msg("catch up block")

			err = h.pbftsm.CatchUp(reply.GetLink())
			if err != nil {
				return xerrors.Errorf("pbft catch up failed: %v", err)
			}
		}
	}

	return h.ack(out, orch)
}

func (h *handler) waitAnnounce(ctx context.Context,
	in mino.Receiver) (*types.SyncMessage, mino.Address, error) {

	for {
		orch, msg, err := in.Recv(ctx)
		if err != nil {
			return nil, nil, xerrors.Errorf("receiver failed: %v", err)
		}

		// The SyncMessage contains the chain to the latest block known by the
		// leader which allows to verify if it is not lying.
		m, ok := msg.(types.SyncMessage)
		if ok {
			return &m, orch, nil
		}
	}
}

func (h *handler) ack(out mino.Sender, orch mino.Address) error {
	// Send the acknowledgement to the orchestrator that the blocks have been
	// caught up.
	err := <-out.Send(types.NewSyncAck(), orch)
	if err != nil {
		return xerrors.Errorf("sending ack failed: %v", err)
	}

	return nil
}

func iter2arr(iter mino.AddressIterator) []mino.Address {
	addrs := []mino.Address{}
	for iter.HasNext() {
		addrs = append(addrs, iter.GetNext())
	}

	return addrs
}
