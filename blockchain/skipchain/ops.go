package skipchain

import (
	"bytes"
	"context"

	"github.com/rs/zerolog"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

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
}

func (ops *operations) insertBlock(block SkipBlock) error {
	err := ops.processor.Validate(block.GetIndex(), block.GetPayload())
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

	fabric.Logger.Trace().Msgf("new block written: %v", block)

	ops.watcher.Notify(block)

	return nil
}

func (ops *operations) catchUp(target SkipBlock, addr mino.Address) error {
	// Target is the block to append to the chain so we check that blocks up to
	// the previous one exist.
	if ops.db.Contains(target.GetIndex() - 1) {
		// Nothing to catch up.
		return nil
	}

	ops.logger.Info().Msg("one or more blocks are missing: starting catch up")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, rcver := ops.rpc.Stream(ctx, mino.NewAddresses(addr))

	err := <-sender.Send(&BlockRequest{To: target.GetPreviousHash()}, addr)
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
