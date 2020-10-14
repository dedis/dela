package pow

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestBlock_New(t *testing.T) {
	block, err := NewBlock(context.Background(), fakeData{}, WithIndex(1), WithNonce(2), WithRoot([]byte{3}))
	require.NoError(t, err)
	require.Equal(t, uint64(1), block.index)
	require.Equal(t, uint64(2), block.nonce)
	require.Equal(t, []byte{3}, block.root)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = NewBlock(ctx, fakeData{})
	require.EqualError(t, err, "couldn't prepare block: context error: context canceled")
}

func TestBlock_Prepare(t *testing.T) {
	block := &Block{
		data: fakeData{},
	}

	ctx := context.Background()

	err := block.prepare(ctx, crypto.NewSha256Factory(), 1)
	require.NoError(t, err)
	require.Len(t, block.hash, 32)

	err = block.prepare(ctx, crypto.NewSha256Factory(), 0)
	require.NoError(t, err)
	require.Len(t, block.hash, 32)

	err = block.prepare(ctx, fake.NewHashFactory(fake.NewBadHash()), 0)
	require.EqualError(t, err, fake.Err("failed to write index"))

	err = block.prepare(ctx, fake.NewHashFactory(fake.NewBadHashWithDelay(1)), 0)
	require.EqualError(t, err, fake.Err("failed to write root"))

	block.data = fakeData{err: fake.GetError()}
	err = block.prepare(ctx, crypto.NewSha256Factory(), 0)
	require.EqualError(t, err, fake.Err("failed to fingerprint data"))

	block.data = fakeData{}
	err = block.prepare(ctx, fake.NewHashFactory(fake.NewBadHashWithDelay(2)), 0)
	require.EqualError(t, err, fake.Err("couldn't marshal digest"))

	err = block.prepare(ctx, fake.NewHashFactory(fake.NewBadHashWithDelay(3)), 0)
	require.EqualError(t, err, fake.Err("couldn't unmarshal digest"))

	err = block.prepare(ctx, fake.NewHashFactory(fake.NewBadHashWithDelay(4)), 0)
	require.EqualError(t, err, fake.Err("failed to write nonce"))
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeData struct {
	validation.Result
	err error
}

func (d fakeData) GetTransactionResults() []validation.TransactionResult {
	return nil
}

func (d fakeData) Fingerprint(io.Writer) error {
	return d.err
}
