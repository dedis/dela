package pow

import (
	"context"
	"encoding/binary"

	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/crypto"
	"golang.org/x/xerrors"
)

// Block is a representation of a batch of transactions for a Proof-of-Work
// consensus. Each block has a fingerprint as a proof of correctness.
type Block struct {
	index uint64
	nonce uint64
	data  validation.Data
}

// NewBlock creates a new block.
func NewBlock(index uint64, nonce uint64, data validation.Data) Block {
	return Block{
		index: index,
		nonce: nonce,
		data:  data,
	}
}

// GetIndex implements ordering.Block. It returns the index of the block.
func (b Block) GetIndex() uint64 {
	return b.index
}

// Prepare is the actual proof of work on the block. It will find the nonce to
// match the difficulty level.
func (b Block) Prepare(ctx context.Context, fac crypto.HashFactory, diff int) (Block, error) {
	h := fac.New()

	indexBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(indexBuf, b.index)
	_, err := h.Write(indexBuf)
	if err != nil {
		return b, xerrors.Errorf("failed to write index: %v", err)
	}

	err = b.data.Fingerprint(h)
	if err != nil {
		return b, xerrors.Errorf("failed to fingerprint data: %v", err)
	}

	var res []byte

	for !checkHash(res, diff) {
		// Allow the proof of work to be aborted at any time if the context is
		// cancelled earlier.
		if ctx.Err() != nil {
			return b, xerrors.Errorf("context error: %v", err)
		}

		// Copy h to get the state before the nonce is written.
		try := h

		b.nonce++

		nonceBuf := make([]byte, 8)
		binary.LittleEndian.PutUint64(nonceBuf, b.nonce)

		_, err = try.Write(nonceBuf)
		if err != nil {
			return b, xerrors.Errorf("failed to write nonce: %v", err)
		}

		res = try.Sum(nil)
	}

	block := NewBlock(b.index, b.nonce, b.data)

	return block, nil
}

func checkHash(hash []byte, diff int) bool {
	n := binary.LittleEndian.Uint64(hash)

	return n < (^uint64(0) >> diff)
}
