package blocksync

import (
	"context"
	"sync/atomic"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/blocksync/types"
	"go.dedis.ch/dela/core/ordering/cosipbft/pbft"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

type result struct {
	err error
	to  mino.Address
}

type defaultSync struct {
	rpc    mino.RPC
	pbftsm pbft.StateMachine
	blocks blockstore.BlockStore
	latest *uint64
}

type SyncParam struct {
	Mino        mino.Mino
	PBFT        pbft.StateMachine
	Blocks      blockstore.BlockStore
	LinkFactory serde.Factory
}

// NewSynchronizer creates a new block synchronizer.
func NewSynchronizer(param SyncParam) (Synchronizer, error) {
	latest := param.Blocks.Len()

	h := &handler{
		latest: &latest,
		blocks: param.Blocks,
		pbftsm: param.PBFT,
	}

	rpc, err := param.Mino.MakeRPC("blocksync", h, types.NewMessageFactory(param.LinkFactory))
	if err != nil {
		return nil, err
	}

	s := defaultSync{
		rpc:    rpc,
		pbftsm: param.PBFT,
		blocks: param.Blocks,
		latest: &latest,
	}

	return s, nil
}

func (s defaultSync) GetLatest() uint64 {
	return atomic.LoadUint64(s.latest)
}

func (s defaultSync) Sync(ctx context.Context, players mino.Players) (<-chan Event, error) {
	ch := make(chan Event, 1)

	go func() {
		err := s.routine(ctx, players, ch)
		if err != nil {
			dela.Logger.Warn().Err(err).Msg("synchronization failed")
		}
	}()

	return ch, nil
}

func (s defaultSync) routine(ctx context.Context, players mino.Players, ch chan Event) error {
	sender, rcvr, err := s.rpc.Stream(ctx, players)
	if err != nil {
		return err
	}

	// 1. Send the announcement message to everyone so that they can learn about
	// the latest block.
	latest := s.blocks.Len()

	errs := sender.Send(types.NewSyncMessage(latest), iter2arr(players.AddressIterator())...)
	for err := range errs {
		if err != nil {
			return err
		}
	}

	// 2. Wait for synchronization request from the followers and start to send
	// them the missing blocks if required.
	soft := map[mino.Address]struct{}{}
	resCh := make(chan result)

	for len(soft) < players.Len() {
		from, msg, err := rcvr.Recv(ctx)
		if err != nil {
			return err
		}

		req, ok := msg.(types.SyncRequest)
		if !ok {
			dela.Logger.Warn().Msg("invalid synchronization message")
			continue
		}

		_, found := soft[from]
		if found {
			dela.Logger.Warn().Msg("found duplicate request")
			continue
		}

		soft[from] = struct{}{}

		sendToChannel(ch, Event{Soft: len(soft)})

		go func() {
			err := s.syncNode(req.GetFrom(), sender, from)

			resCh <- result{
				err: err,
				to:  from,
			}
		}()
	}

	dela.Logger.Debug().Msg("soft synchronization done")

	// 3. Wait for the hard synchronization to be done.
	hard := map[mino.Address]struct{}{}
	syncErrs := []error{}

	for len(hard) < len(soft) {
		res := <-resCh
		hard[res.to] = struct{}{}

		if res.err != nil {
			syncErrs = append(syncErrs, res.err)
		}

		sendToChannel(ch, Event{
			Soft:   len(soft),
			Hard:   len(hard),
			Errors: syncErrs,
		})
	}

	dela.Logger.Debug().Msg("hard synchronization done")

	return nil
}

func (s defaultSync) syncNode(from uint64, sender mino.Sender, to mino.Address) error {
	for i := from; i < s.blocks.Len(); i++ {
		dela.Logger.Debug().Uint64("index", i).Str("to", to.String()).Msg("send block")

		link, err := s.blocks.GetByIndex(i)
		if err != nil {
			return err
		}

		err = <-sender.Send(types.NewSyncReply(link), to)
		if err != nil {
			return err
		}
	}

	return nil
}

type handler struct {
	mino.UnsupportedHandler

	latest *uint64
	blocks blockstore.BlockStore
	pbftsm pbft.StateMachine
}

func (h *handler) Stream(out mino.Sender, in mino.Receiver) error {
	ctx := context.Background()

	orch, msg, err := in.Recv(ctx)
	if err != nil {
		return err
	}

	m, ok := msg.(types.SyncMessage)
	if !ok {
		return nil
	}

	dela.Logger.Debug().Uint64("index", m.GetLatestIndex()).Msg("received synchronization message")

	err = <-out.Send(types.NewSyncRequest(h.blocks.Len()), orch)
	if err != nil {
		return err
	}

	curr := atomic.LoadUint64(h.latest)
	if m.GetLatestIndex() > *h.latest {
		if !atomic.CompareAndSwapUint64(h.latest, curr, m.GetLatestIndex()) {
			// Another thread updated the latest index so we stop this one.
			return nil
		}
	}

	for h.blocks.Len() < m.GetLatestIndex() {
		_, msg, err := in.Recv(ctx)
		if err != nil {
			return err
		}

		reply, ok := msg.(types.SyncReply)
		if ok {
			dela.Logger.Debug().
				Uint64("index", reply.GetLink().GetTo().GetIndex()).
				Msg("catch up block")

			err = h.pbftsm.CatchUp(reply.GetLink())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func sendToChannel(ch chan Event, evt Event) {
	empty := false
	for !empty {
		// Drain the channel first
		select {
		case <-ch:
		default:
			empty = true
		}
	}

	ch <- evt
}

func iter2arr(iter mino.AddressIterator) []mino.Address {
	addrs := []mino.Address{}
	for iter.HasNext() {
		addrs = append(addrs, iter.GetNext())
	}

	return addrs
}
