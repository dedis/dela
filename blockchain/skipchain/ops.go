package skipchain

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

const catchUpLeeway = 100 * time.Millisecond

// operations implements helper functions that can be used by the handlers for
// common operations.
type operations struct {
	logger          zerolog.Logger
	addr            mino.Address
	blockFactory    BlockFactory
	responseFactory serde.Factory
	db              Database
	rpc             mino.RPC
	watcher         blockchain.Observable
	reactor         blockchain.Reactor

	catchUpLock sync.Mutex
}

func (ops *operations) commitBlock(block SkipBlock) error {
	err := ops.db.Atomic(func(tx Queries) error {
		err := tx.Write(block)
		if err != nil {
			return xerrors.Errorf("couldn't write block: %v", err)
		}

		err = ops.reactor.InvokeCommit(block.GetPayload())
		if err != nil {
			return xerrors.Errorf("couldn't commit block: %v", err)
		}

		return nil
	})

	if err != nil {
		return xerrors.Errorf("tx failed: %v", err)
	}

	dela.Logger.Trace().Msgf("new block written: %v", block)

	ops.watcher.Notify(block)

	return nil
}

func (ops *operations) catchUp(index uint64, addr mino.Address) error {
	ops.catchUpLock.Lock()
	defer ops.catchUpLock.Unlock()

	if index == 0 {
		return nil
	}

	// Target is the block to append to the chain so we check that blocks up to
	// the previous one exist.
	if ops.db.Contains(index - 1) {
		// Nothing to catch up.
		return nil
	}

	from := uint64(0)
	if ops.db.Contains(0) {
		last, err := ops.db.ReadLast()
		if err != nil {
			return xerrors.Errorf("couldn't read last block: %v", err)
		}

		from = last.GetIndex() + 1
	}

	if index-from <= 1 {
		// When only one block is missing, that probably means the propagation
		// is not yet over, so it gives a chance to wait for it before starting
		// the actual catch up.
		ops.waitBlock(index - 1)

		// Check again after the lock is acquired again.
		if ops.db.Contains(index - 1) {
			return nil
		}
	}

	ops.logger.Info().
		Str("addr", ops.addr.String()).
		Msg("one or more blocks are missing: starting catch up")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, rcver, err := ops.rpc.Stream(ctx, mino.NewAddresses(addr))
	if err != nil {
		return xerrors.Errorf("couldn't open stream: %v", err)
	}

	req := BlockRequest{
		from: from,
		to:   index - 1,
	}

	err = <-sender.Send(req, addr)
	if err != nil {
		return xerrors.Errorf("couldn't send block request: %v", err)
	}

	for {
		_, msg, err := rcver.Recv(ctx)
		if err != nil {
			return xerrors.Errorf("couldn't receive message: %v", err)
		}

		resp, ok := msg.(BlockResponse)
		if !ok {
			return xerrors.Errorf("invalid response type '%T' != '%T'", msg, resp)
		}

		err = ops.commitBlock(resp.block)
		if err != nil {
			return xerrors.Errorf("couldn't store block: %v", err)
		}

		if resp.block.GetIndex() >= index-1 {
			return nil
		}
	}
}

// waitBlock releases the catch up lock and wait for a block with the given
// index to be committed. If it does not arrive after a given amount if time,
// the catch up will start anyway.
//
// Note: it expects the lock to be acquired before and released later.
func (ops *operations) waitBlock(index uint64) {
	ops.catchUpLock.Unlock()
	defer ops.catchUpLock.Lock()

	observer := skipchainObserver{
		ch: make(chan blockchain.Block, 1),
	}

	ops.watcher.Add(observer)
	defer ops.watcher.Remove(observer)

	for {
		select {
		case block := <-observer.ch:
			if block.GetIndex() == index {
				return
			}
		case <-time.After(catchUpLeeway):
			// Even if the commit message could arrive later, the catch up procedure
			// starts anyway.
			return
		}
	}
}
