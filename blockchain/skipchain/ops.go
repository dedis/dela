package skipchain

import (
	"bytes"
	"context"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

type operations struct {
	encoder      encoding.ProtoMarshaler
	addr         mino.Address
	processor    blockchain.PayloadProcessor
	blockFactory blockFactory
	db           Database
	rpc          mino.RPC
	watcher      blockchain.Observable
}

func (ops *operations) insertBlock(in proto.Message) (SkipBlock, error) {
	block, err := ops.blockFactory.decodeBlock(in)
	if err != nil {
		return block, xerrors.Errorf("couldn't decode block: %v", err)
	}

	err = ops.processor.Validate(block.Index, block.GetPayload())
	if err != nil {
		return block, xerrors.Errorf("couldn't validate block: %v", err)
	}

	err = ops.commitBlock(block)
	if err != nil {
		// No wrapping to avoid redundancy.
		return block, err
	}

	return block, nil
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

	fabric.Logger.Info().Msg("one or more blocks are missing: starting catch up")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, rcver := ops.rpc.Stream(ctx, newRoster(addr))

	sender.Send(&BlockRequest{To: target.GetPreviousHash()}, addr)

	for {
		_, msg, err := rcver.Recv(ctx)
		if err != nil {
			return xerrors.Errorf("couldn't receive message: %v", err)
		}

		resp, ok := msg.(*BlockResponse)
		if !ok {
			return xerrors.Errorf("invalid response type '%T' != '%T'", msg, resp)
		}

		block, err := ops.insertBlock(resp.GetBlock())
		if err != nil {
			return xerrors.Errorf("couldn't store block: %v", err)
		}

		if bytes.Equal(block.GetHash(), target.GetPreviousHash()) {
			return nil
		}
	}
}
