package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNonBlockingQueue_Push(t *testing.T) {
	queue := newNonBlockingQueue()
	require.Len(t, queue.buffer, 0)
	require.Equal(t, 0, cap(queue.buffer))

	queue.cap = 1
	queue.limit = 3 // 2^3*1 = 8 is the maximum capacity
	queue.running = true

	// First packet goes to the channel.
	require.NoError(t, queue.Push(fakePkt{}))
	require.Equal(t, 0, cap(queue.buffer))

	require.NoError(t, queue.Push(fakePkt{}))
	require.Equal(t, 1, cap(queue.buffer))

	require.NoError(t, queue.Push(fakePkt{}))
	require.Equal(t, 2, cap(queue.buffer))

	require.NoError(t, queue.Push(fakePkt{}))
	require.Equal(t, 4, cap(queue.buffer))

	require.NoError(t, queue.Push(fakePkt{}))
	require.Equal(t, 4, cap(queue.buffer))

	require.NoError(t, queue.Push(fakePkt{}))
	require.Equal(t, 8, cap(queue.buffer))

	require.NoError(t, queue.Push(fakePkt{}))
	require.NoError(t, queue.Push(fakePkt{}))
	require.NoError(t, queue.Push(fakePkt{}))
	err := queue.Push(fakePkt{})
	require.EqualError(t, err, "queue is at maximum capacity")

	queue.running = true
	go queue.pushAndWait()

	for i := 0; i < 9; i++ {
		waitPkt(t, queue)
	}

	require.Equal(t, 0, cap(queue.buffer))
}

// -----------------------------------------------------------------------------
// Utility functions

func waitPkt(t *testing.T, queue Queue) {
	select {
	case <-queue.Channel():
	case <-time.After(time.Second):
		t.Fatal("packet expected")
	}
}
