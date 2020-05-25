package minogrpc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNonBlockingQueue_Push(t *testing.T) {
	queue := newNonBlockingQueue()

	n := 255

	for i := 0; i < n; i++ {
		queue.Push(&Message{From: []byte{byte(i)}})
	}

	for i := 0; i < n; i++ {
		msg := <-queue.Channel()
		require.NotNil(t, msg)
		// Make sure it comes in order.
		require.Equal(t, []byte{byte(i)}, msg.GetFrom())
	}

	queue.working.Wait()

	require.False(t, queue.running)
	require.Len(t, queue.buffer, 0)
}
