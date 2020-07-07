package skipchain

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/blockchain/skipchain/types"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestNoBlockError_Is(t *testing.T) {
	err := NewNoBlockError(0)
	require.True(t, xerrors.Is(err, NewNoBlockError(0)))
	require.False(t, xerrors.Is(err, NewNoBlockError(1)))
	require.False(t, xerrors.Is(err, xerrors.New("oops")))
}

func TestInMemoryDatabase_Write(t *testing.T) {
	db := NewInMemoryDatabase()

	err := db.Write(makeBlock(t))
	require.NoError(t, err)

	err = db.Write(makeBlock(t, types.WithIndex(1)))
	require.NoError(t, err)

	err = db.Write(makeBlock(t, types.WithIndex(1)))
	require.NoError(t, err)

	err = db.Write(makeBlock(t, types.WithIndex(5)))
	require.EqualError(t, err, "missing intermediate blocks for index 5")
}

func TestInMemoryDatabase_Read(t *testing.T) {
	db := NewInMemoryDatabase()
	db.blocks = []types.SkipBlock{
		makeBlock(t, types.WithIndex(0)),
		makeBlock(t, types.WithIndex(1)),
	}

	block, err := db.Read(1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), block.Index)

	_, err = db.Read(5)
	require.EqualError(t, err, "block at index 5 not found")
}

func TestInMemoryDatabase_ReadLast(t *testing.T) {
	db := NewInMemoryDatabase()
	db.blocks = []types.SkipBlock{
		makeBlock(t, types.WithIndex(0)),
		makeBlock(t, types.WithIndex(1)),
	}

	block, err := db.ReadLast()
	require.NoError(t, err)
	require.Equal(t, uint64(1), block.Index)

	db.blocks = append(db.blocks, makeBlock(t, types.WithIndex(2)))
	block, err = db.ReadLast()
	require.NoError(t, err)
	require.Equal(t, uint64(2), block.Index)

	db.blocks = nil
	_, err = db.ReadLast()
	require.EqualError(t, err, "database is empty")
}

func TestInMemoryDatabase_Atomic(t *testing.T) {
	db := NewInMemoryDatabase()

	err := db.Atomic(func(ops Queries) error {
		// Make sure you can still use out of tx operations.
		require.NoError(t, db.Write(makeBlock(t, types.WithIndex(0))))
		return ops.Write(makeBlock(t, types.WithIndex(0)))
	})
	require.NoError(t, err)
	require.Len(t, db.blocks, 1)

	err = db.Atomic(func(ops Queries) error {
		return ops.Write(makeBlock(t, types.WithIndex(2)))
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "couldn't execute transaction: ")
	require.Len(t, db.blocks, 1)
}

// -----------------------------------------------------------------------------
// Utility functions

func makeBlock(t *testing.T, opts ...types.SkipBlockOption) types.SkipBlock {
	block, err := types.NewSkipBlock(fakePayload{}, opts...)
	require.NoError(t, err)

	return block
}

type fakePayload struct{}

func (pl fakePayload) Serialize(serde.Context) ([]byte, error) {
	return []byte(`{}`), nil
}

func (pl fakePayload) Fingerprint(io.Writer) error {
	return nil
}
