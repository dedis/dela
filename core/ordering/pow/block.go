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

// Difficulty is the default difficulty.
const Difficulty = 1

// Block is a representation of a batch of transactions for a Proof-of-Work
// consensus. Each block has a fingerprint as a proof of correctness.
type Block struct {
	index uint64
	nonce uint64
	root  []byte
	data  validation.Result
	hash  []byte
}

type blockTemplate struct {
	Block

	hashFactory crypto.HashFactory
	difficulty  uint
}

// BlockOption is the type of options to create a block.
type BlockOption func(*blockTemplate)

// WithIndex is an option to set the block index.
func WithIndex(index uint64) BlockOption {
	return func(tmpl *blockTemplate) {
		tmpl.index = index
	}
}

// WithNonce is an option to set the nonce of a block.
func WithNonce(nonce uint64) BlockOption {
	return func(tmpl *blockTemplate) {
		tmpl.nonce = nonce
		tmpl.difficulty = 0
	}
}

// WithRoot is an option to set the root of a block.
func WithRoot(root []byte) BlockOption {
	return func(tmpl *blockTemplate) {
		tmpl.root = root
	}
}

// WithDifficulty is an option to set the difficulty of the proof of work. If
// the difficulty is set to 0, the hash will be calculated according to the
// current nonce.
func WithDifficulty(diff uint) BlockOption {
	return func(tmpl *blockTemplate) {
		tmpl.difficulty = diff
	}
}

// NewBlock creates a new block.
func NewBlock(ctx context.Context, data validation.Result, opts ...BlockOption) (Block, error) {
	tmpl := blockTemplate{
		Block: Block{
			data: data,
		},
		hashFactory: crypto.NewSha256Factory(),
		difficulty:  Difficulty,
	}

	for _, opt := range opts {
		opt(&tmpl)
	}

	err := tmpl.Block.prepare(ctx, tmpl.hashFactory, tmpl.difficulty)
	if err != nil {
		return tmpl.Block, xerrors.Errorf("couldn't prepare block: %v", err)
	}

	return tmpl.Block, nil
}

// Prepare is the actual proof of work on the block. It will find the nonce to
// match the difficulty level.
func (b *Block) prepare(ctx context.Context, fac crypto.HashFactory, diff uint) error {
	h := fac.New()

	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, b.index)
	_, err := h.Write(buffer)
	if err != nil {
		return xerrors.Errorf("failed to write index: %v", err)
	}

	_, err = h.Write(b.root)
	if err != nil {
		return xerrors.Errorf("failed to write root: %v", err)
	}

	err = b.data.Fingerprint(h)
	if err != nil {
		return xerrors.Errorf("failed to fingerprint data: %v", err)
	}

	// The state before writing the nonce is saved so it does not need to be
	// computed all the time.
	inter, err := h.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		return xerrors.Errorf("couldn't marshal digest: %v", err)
	}

	bitstring := make([]byte, h.Size())
	for i := range bitstring {
		bitstring[i] = 0xff
	}

	target := new(big.Int)
	target.SetBytes(bitstring)
	target.Rsh(target, diff)

	for {
		// Allow the proof of work to be aborted at any time if the context is
		// cancelled earlier.
		if ctx.Err() != nil {
			return xerrors.Errorf("context error: %v", ctx.Err())
		}

		// Copy h to get the state before the nonce is written.
		err := h.(encoding.BinaryUnmarshaler).UnmarshalBinary(inter)
		if err != nil {
			return xerrors.Errorf("couldn't unmarshal digest: %v", err)
		}

		binary.LittleEndian.PutUint64(buffer, b.nonce)
		_, err = h.Write(buffer)
		if err != nil {
			return xerrors.Errorf("failed to write nonce: %v", err)
		}

		res := h.Sum(nil)
		// If no difficulty is set, the provided nonce defines the hash,
		// otherwise it looks for a hash that matches the difficulty.
		if diff == 0 || matchDifficulty(res, target) {
			b.hash = res
			return nil
		}

		b.nonce++
	}
}

func matchDifficulty(hash []byte, limit *big.Int) bool {
	value := new(big.Int)
	value.SetBytes(hash)

	return value.Cmp(limit) == -1
}
