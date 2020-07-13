package pow

import (
	"context"
	"encoding"
	"encoding/binary"
	"math/big"

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
	hash  []byte
}

type blockTemplate struct {
	Block

	hashFactory crypto.HashFactory
	difficulty  int
}

// BlockOption is the type of options to create a block.
type BlockOption func(*blockTemplate)

// WithIndex is an option to set the block index.
func WithIndex(index uint64) BlockOption {
	return func(b *blockTemplate) {
		b.index = index
	}
}

// WithNonce is an option to set the nonce of a block.
func WithNonce(nonce uint64) BlockOption {
	return func(b *blockTemplate) {
		b.nonce = nonce
	}
}

// NewBlock creates a new block.
func NewBlock(ctx context.Context, data validation.Data, opts ...BlockOption) (Block, error) {
	tmpl := blockTemplate{
		Block: Block{
			data: data,
		},
		hashFactory: crypto.NewSha256Factory(),
	}

	for _, opt := range opts {
		opt(&tmpl)
	}

	err := tmpl.Block.prepare(ctx, tmpl.hashFactory, tmpl.difficulty)
	if err != nil {
		return tmpl.Block, err
	}

	return tmpl.Block, nil
}

// GetIndex implements ordering.Block. It returns the index of the block.
func (b Block) GetIndex() uint64 {
	return b.index
}

// Prepare is the actual proof of work on the block. It will find the nonce to
// match the difficulty level.
func (b *Block) prepare(ctx context.Context, fac crypto.HashFactory, diff int) error {
	h := fac.New()

	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, b.index)
	_, err := h.Write(buffer)
	if err != nil {
		return xerrors.Errorf("failed to write index: %v", err)
	}

	err = b.data.Fingerprint(h)
	if err != nil {
		return xerrors.Errorf("failed to fingerprint data: %v", err)
	}

	// The state before writing the nonce is saved so it does not need to be
	// computed all the time.
	inter, err := h.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		return err
	}

	var res []byte
	target := new(big.Int)
	target.SetBit(target, 256-diff, 1)

	nonce := b.nonce
	digest := fac.New()

	for !checkHash(res, target) {
		// Allow the proof of work to be aborted at any time if the context is
		// cancelled earlier.
		if ctx.Err() != nil {
			return xerrors.Errorf("context error: %v", ctx.Err())
		}

		// Copy h to get the state before the nonce is written.
		err := digest.(encoding.BinaryUnmarshaler).UnmarshalBinary(inter)
		if err != nil {
			return err
		}

		nonce++

		binary.LittleEndian.PutUint64(buffer, nonce)

		_, err = digest.Write(buffer)
		if err != nil {
			return xerrors.Errorf("failed to write nonce: %v", err)
		}

		res = digest.Sum(nil)
	}

	b.nonce = nonce
	b.hash = res

	return nil
}

func checkHash(hash []byte, limit *big.Int) bool {
	if len(hash) == 0 {
		return false
	}

	value := new(big.Int)
	value.SetBytes(hash)

	return value.Cmp(limit) == -1
}
