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

	queue.cap = 2
	queue.limit = 3 // 2^3*2 = 16 is the maximum capacity
	queue.running = true

	// First packet goes to the channel.
	sendPkt(t, queue, 1)
	require.Equal(t, 0, cap(queue.buffer))

	sendPkt(t, queue, 1)
	require.Equal(t, 2, cap(queue.buffer))

	sendPkt(t, queue, 2)
	require.Equal(t, 4, cap(queue.buffer))

	sendPkt(t, queue, 2)
	require.Equal(t, 8, cap(queue.buffer))

	sendPkt(t, queue, 4)
	require.Equal(t, 16, cap(queue.buffer))

	sendPkt(t, queue, 7)
	err := queue.Push(fakePkt{})
	require.EqualError(t, err, "queue is at maximum capacity")

	queue.running = true
	go queue.pushAndWait()

	for i := 0; i < 17; i++ {
		waitPkt(t, queue)
	}

	require.Equal(t, 0, cap(queue.buffer))
}

// -----------------------------------------------------------------------------
// Utility functions

func sendPkt(t *testing.T, queue Queue, n int) {
	for i := 0; i < n; i++ {
		require.NoError(t, queue.Push(fakePkt{}))
	}
}

func waitPkt(t *testing.T, queue Queue) {
	select {
	case <-queue.Channel():
	case <-time.After(time.Second):
		t.Fatal("packet expected")
	}
}
