package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSimpleQueue_Len(t *testing.T) {
	queue := NewQueue().(*simpleQueue)

	queue.buffer[Digest{}] = SkipBlock{}
	queue.buffer[Digest{1}] = SkipBlock{}

	require.Equal(t, 2, queue.Len())
}

func TestSimpleQueue_Get(t *testing.T) {
	queue := NewQueue().(*simpleQueue)

	id := Digest{1}
	queue.buffer[id] = SkipBlock{Index: 4}

	block, found := queue.Get(id[:])
	require.True(t, found)
	require.Equal(t, uint64(4), block.GetIndex())

	unknown := Digest{2}
	_, found = queue.Get(unknown[:])
	require.False(t, found)

	_, found = queue.Get(nil)
	require.False(t, found)
}

func TestSimpleQueue_Add(t *testing.T) {
	queue := NewQueue().(*simpleQueue)

	queue.Add(SkipBlock{})
	queue.Add(SkipBlock{hash: Digest{1}})
	require.Len(t, queue.buffer, 2)
}

func TestSimpleQueue_Clear(t *testing.T) {
	queue := NewQueue().(*simpleQueue)

	queue.buffer[Digest{}] = SkipBlock{}
	queue.buffer[Digest{1}] = SkipBlock{}

	queue.Clear()
	require.Len(t, queue.buffer, 0)
}
