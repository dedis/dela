package skipchain

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

const catchUpLeeway = 100 * time.Millisecond

// operations implements helper functions that can be used by the handlers for
// common operations.
type operations struct {
	logger       zerolog.Logger
	encoder      encoding.ProtoMarshaler
	addr         mino.Address
	processor    blockchain.PayloadProcessor
	blockFactory blockFactory
	db           Database
	rpc          mino.RPC
	watcher      blockchain.Observable
	consensus    consensus.Consensus

	catchUpLock sync.Mutex
}

func (ops *operations) insertBlock(block SkipBlock) error {
	err := ops.processor.Validate(block.Origin, block.GetPayload())
	if err != nil {
		return xerrors.Errorf("couldn't validate block: %v", err)
	}

	err = ops.commitBlock(block)
	if err != nil {
		// No wrapping to avoid redundancy.
		return err
	}

	return nil
}

func (ops *operations) commitBlock(block SkipBlock) error {
	err := ops.db.Atomic(func(tx Queries) error {
		err := tx.Write(block)
		if err != nil {
			return xerrors.Errorf("couldn't write block: %v", err)
		}

		err = ops.processor.Commit(block.GetPayload())
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

func (ops *operations) catchUp(target SkipBlock, addr mino.Address) error {
	ops.catchUpLock.Lock()
	defer ops.catchUpLock.Unlock()

	// Target is the block to append to the chain so we check that blocks up to
	// the previous one exist.
	if ops.db.Contains(target.GetIndex() - 1) {
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

	if target.GetIndex()-from <= 1 {
		// When only one block is missing, that probably means the propagation
		// is not yet over, so it gives a chance to wait for it before starting
		// the actual catch up.
		ops.waitBlock(target.GetIndex() - 1)

		// Check again after the lock is acquired again.
		if ops.db.Contains(target.GetIndex() - 1) {
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

	req := &BlockRequest{
		From: from,
		To:   target.GetIndex() - 1,
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

		resp, ok := msg.(*BlockResponse)
		if !ok {
			return xerrors.Errorf("invalid response type '%T' != '%T'", msg, resp)
		}

		block, err := ops.blockFactory.decodeBlock(resp.GetBlock())
		if err != nil {
			return xerrors.Errorf("couldn't decode block: %v", err)
		}

		if resp.GetChain() != nil {
			factory, err := ops.consensus.GetChainFactory()
			if err != nil {
				return xerrors.Errorf("couldn't get chain factory: %v", err)
			}

			chain, err := factory.FromProto(resp.GetChain())
			if err != nil {
				return xerrors.Errorf("couldn't decode chain: %v", err)
			}

			if !bytes.Equal(chain.GetLastHash(), block.GetHash()) {
				return xerrors.Errorf("mismatch chain: hash '%x' != '%x'",
					chain.GetLastHash(), block.GetHash())
			}

			err = ops.consensus.Store(chain)
			if err != nil {
				return xerrors.Errorf("couldn't store chain: %v", err)
			}
		} else if block.GetIndex() != 0 {
			return xerrors.New("missing chain to the block in the response")
		}

		err = ops.insertBlock(block)
		if err != nil {
			return xerrors.Errorf("couldn't store block: %v", err)
		}

		if bytes.Equal(block.GetHash(), target.GetPreviousHash()) {
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
