package skipchain

import (
	"testing"

	"github.com/stretchr/testify/require"
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

	err := db.Write(SkipBlock{})
	require.NoError(t, err)

	err = db.Write(SkipBlock{Index: 1})
	require.NoError(t, err)

	err = db.Write(SkipBlock{Index: 1})
	require.NoError(t, err)

	err = db.Write(SkipBlock{Index: 5})
	require.EqualError(t, err, "missing intermediate blocks for index 5")
}

func TestInMemoryDatabase_Read(t *testing.T) {
	db := NewInMemoryDatabase()
	db.blocks = []SkipBlock{
		{Index: 0},
		{Index: 1},
	}

	block, err := db.Read(1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), block.Index)

	_, err = db.Read(5)
	require.EqualError(t, err, "block at index 5 not found")
}

func TestInMemoryDatabase_ReadLast(t *testing.T) {
	db := NewInMemoryDatabase()
	db.blocks = []SkipBlock{
		{Index: 0},
		{Index: 1},
	}

	block, err := db.ReadLast()
	require.NoError(t, err)
	require.Equal(t, uint64(1), block.Index)

	db.blocks = append(db.blocks, SkipBlock{Index: 2})
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
		require.NoError(t, db.Write(SkipBlock{Index: 0}))
		return ops.Write(SkipBlock{Index: 0})
	})
	require.NoError(t, err)
	require.Len(t, db.blocks, 1)

	err = db.Atomic(func(ops Queries) error {
		return ops.Write(SkipBlock{Index: 2})
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "couldn't execute transaction: ")
	require.Len(t, db.blocks, 1)
}
