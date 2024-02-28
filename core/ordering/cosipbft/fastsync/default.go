package fastsync

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/blocksync"
	"go.dedis.ch/dela/core/ordering/cosipbft/fastsync/types"
	"go.dedis.ch/dela/core/ordering/cosipbft/pbft"
	otypes "go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/tracing"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

var timeoutSync = 20 * time.Second
var protocolName = "fastsync"

// fastSync is a block synchronizer which quickly catches up to the
// latest block.
//
// - implements fastsync.Synchronizer
type fastSync struct {
	logger zerolog.Logger
	rpc    mino.RPC
	pbftsm pbft.StateMachine
	blocks blockstore.BlockStore

	Mino mino.Mino

	latest      *uint64
	catchUpLock *sync.Mutex

	// This is for debugging
	syncMessages *int
}

// NewSynchronizer creates a new block synchronizer.
func NewSynchronizer(param blocksync.SyncParam) Synchronizer {
	latest := param.Blocks.Len()

	logger := dela.Logger.With().Str("addr", param.Mino.GetAddress().String()).Logger()

	h := &handler{
		latest:      &latest,
		catchUpLock: new(sync.Mutex),
		logger:      logger,
		genesis:     param.Genesis,
		blocks:      param.Blocks,
		pbftsm:      param.PBFT,
		verifierFac: param.VerifierFactory,
	}

	fac := types.NewMessageFactory(param.LinkFactory)

	s := fastSync{
		logger:      logger,
		rpc:         mino.MustCreateRPC(param.Mino, "fastsync", h, fac),
		pbftsm:      param.PBFT,
		blocks:      param.Blocks,
		latest:      &latest,
		catchUpLock: h.catchUpLock,
		Mino:        param.Mino,
	}

	return s
}

// Sync implements fastsync.Synchronizer.
// It asks the other nodes what their latest block is, and then chooses some
// nodes randomly to request catching up the missing blocks.
func (s fastSync) Sync(ctx context.Context, players mino.Players, config Config) error {
	if players.Len() == 0 {
		return xerrors.Errorf("need at least 1 node to contact")
	}
	ctx = context.WithValue(ctx, tracing.ProtocolKey, protocolName)
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(timeoutSync))
	defer cancel()

	// Make sure that the address of this node is at the beginning of the list.
	addresses := []mino.Address{s.Mino.GetAddress()}
	for iter := players.AddressIterator(); iter.HasNext(); {
		addr := iter.GetNext()
		if !s.Mino.GetAddress().Equal(addr) {
			addresses = append(addresses, addr)
		}
	}
	players = mino.NewAddresses(addresses...)
	sender, rcvr, err := s.rpc.Stream(ctx, players)
	if err != nil {
		return xerrors.Errorf("stream failed: %v", err)
	}

	// Send a catchup-request to f+1 nodes with our latest known block,
	// but not to this node.
	// This should be enough, because the protocol supposes there are only
	// f byzantine nodes, so this should contact at least one healthy node.
	f := (players.Len() - 1) / 3
	nodes := players.Take(mino.RangeFilter(1, players.Len()),
		mino.RandomFilter(f+1))

	// Send the request as many times as needed, because with a
	// SplitMessageSize < size(all missing blocks), multiple requests are
	// needed.
	blockCount := s.blocks.Len()
	for {
		more, err := s.requestSync(ctx, sender, rcvr, nodes, config.SplitMessageSize)
		if err != nil {
			return xerrors.Errorf("error while requesting sync updates: %v", err)
		}
		if s.syncMessages != nil {
			*s.syncMessages += 1
		}

		if !more {
			break
		} else if blockCount == s.blocks.Len() {
			s.logger.Warn().Msgf("one of the nodes returned it has more blocks, "+
				"but didn't deliver: %v", nodes)
			break
		}

		blockCount = s.blocks.Len()
	}

	return nil
}

// requestSync asks all 'nodes' to send their block updates.
// The return is a boolean indicating whether at least one node indicated
// there are more blocks.
// This might be wrong, as there is no check whether the sending node is
// byzantine or not.
func (s fastSync) requestSync(
	ctx context.Context,
	sender mino.Sender,
	rcvr mino.Receiver,
	nodes mino.Players,
	sms uint64,
) (bool, error) {
	// Send the messages to all nodes
	s.logger.Debug().Msgf("Sending catchup req to %+v", nodes)
	errs := sender.Send(types.NewRequestCatchupMessage(sms, s.blocks.Len()),
		iter2arr(nodes.AddressIterator())...)
	errCount := 0
	for err := range errs {
		errCount += 1
		if err != nil {
			s.logger.Warn().Err(err).Msgf("announcement failed to one node")
		}
	}
	if errCount == nodes.Len() {
		return false, xerrors.Errorf("contacted %d nodes, but all failed: %v", len(errs), errs)
	}

	// Wait for all replies, supposing that there are no more than f nodes
	// not replying or replying with wrong blocks.

	replies := make(map[string]struct{})
	moreBlocks := false
	for len(replies) < nodes.Len() {
		s.logger.Debug().Msgf("Waiting for replies: %d < %d", len(replies), nodes.Len())
		from, msg, err := rcvr.Recv(ctx)
		if err == context.Canceled || err == context.DeadlineExceeded || err == io.EOF {
			return moreBlocks, nil
		}
		if err != nil {
			s.logger.Debug().Err(err).Msg("sync finished with error")
			return false, nil
		}

		_, received := replies[from.String()]
		if received {
			s.logger.Warn().Msgf("received two fastsync messages from %s", from)
			continue
		}

		catchup, ok := msg.(types.CatchupMessage)
		if ok {
			s.logger.Info().Msgf("Got %d fastsync blocks from %v",
				len(catchup.GetBlockLinks()), from)

			replies[from.String()] = struct{}{}
			moreBlocks = moreBlocks || catchup.GetSplitMessage()

			for _, bl := range catchup.GetBlockLinks() {
				if bl.GetBlock().GetIndex() >= s.blocks.Len() {
					err := s.pbftsm.CatchUp(bl)
					if err != nil {
						s.logger.Warn().Err(err).Msg("while using block to catchup")
					}
				}
			}
		}
	}

	s.logger.Info().Msgf("Currently have %d blocks", s.blocks.Len())

	return moreBlocks, nil
}

// handler is a Mino handler for the synchronization messages.
//
// - implements mino.Handler
type handler struct {
	mino.UnsupportedHandler

	latest      *uint64
	catchUpLock *sync.Mutex

	logger      zerolog.Logger
	blocks      blockstore.BlockStore
	genesis     blockstore.GenesisStore
	pbftsm      pbft.StateMachine
	verifierFac crypto.VerifierFactory
}

// Stream implements mino.Handler. It waits for a request message and then
// replies with eventually missing BlockLinks of the requester.
func (h *handler) Stream(out mino.Sender, in mino.Receiver) error {
	h.logger.Debug().Msg("Starting stream")
	ctx := context.Background()
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(timeoutSync))
	defer cancel()

	for sentAllBlocks := false; !sentAllBlocks; {
		m, orch, err := h.waitRequest(ctx, in)
		if err != nil {
			return xerrors.Errorf("no request: %v", err)
		}

		blReply, err := h.getBlocks(m)
		if err != nil {
			return xerrors.Errorf("creating blocks to send failed: %v", err)
		}

		sentAllBlocks = m.GetLatest()+uint64(len(blReply)) >= h.blocks.Len()
		err = <-out.Send(types.NewCatchupMessage(
			!sentAllBlocks, blReply), orch)
		if err != nil {
			return xerrors.Errorf("sending request failed: %v", err)
		}
	}

	h.logger.Debug().Msg("done sending catchup blocks")

	return nil
}

// getBlocks creates a reply that will only overflow the given message-size
// budget by at most one block.
func (h *handler) getBlocks(m *types.RequestCatchupMessage) ([]otypes.BlockLink, error) {
	var blReply []otypes.BlockLink

	msgSize := uint64(0)
	if h.blocks.Len() > m.GetLatest() {
		for index := m.GetLatest(); index < h.blocks.Len(); index++ {
			bl, err := h.blocks.GetByIndex(index)
			if err != nil {
				return blReply, xerrors.Errorf("failed to get block with index %d", index)
			}
			blReply = append(blReply, bl)
			b, err := bl.Serialize(json.NewContext())
			if err != nil {
				return blReply, xerrors.Errorf("failed to serialize block %d: %v", index, err)
			}
			msgSize += uint64(len(b))

			if m.GetSplitMessageSize() > 0 && msgSize >= m.GetSplitMessageSize() {
				h.logger.Debug().Msgf("splitting message because size %d >= %d",
					msgSize, m.GetSplitMessageSize())
				break
			}
		}
		h.logger.Debug().Msgf("Sending blocks %d..%d", m.GetLatest(),
			m.GetLatest()+uint64(len(blReply))-1)
	} else {
		h.logger.Debug().Msgf("No new blocks to send")
	}

	return blReply, nil
}

func (h *handler) waitRequest(
	ctx context.Context,
	in mino.Receiver,
) (*types.RequestCatchupMessage, mino.Address, error) {

	for {
		orch, msg, err := in.Recv(ctx)
		if err != nil {
			return nil, nil, xerrors.Errorf("receiver failed: %v", err)
		}

		// The SyncMessage contains the chain to the latest block known by the
		// leader which allows to verify if it is not lying.
		m, ok := msg.(types.RequestCatchupMessage)
		if ok {
			return &m, orch, nil
		}
	}
}

func iter2arr(iter mino.AddressIterator) []mino.Address {
	var addrs []mino.Address
	for iter.HasNext() {
		addrs = append(addrs, iter.GetNext())
	}

	return addrs
}
