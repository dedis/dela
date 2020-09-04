package minogrpc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
)

func TestNonBlockingQueue_Push(t *testing.T) {
	queue := newNonBlockingQueue()

	n := 255

	for i := 0; i < n; i++ {
		queue.Push(fakePacket{dest: []mino.Address{fake.NewAddress(0)}})
	}

	for i := 0; i < n; i++ {
		msg := <-queue.Channel()
		require.NotNil(t, msg)
		// Make sure it comes in order.
		require.Equal(t, fake.NewAddress(0), msg.GetDestination()[0])
	}

	queue.working.Wait()

	require.False(t, queue.running)
	require.Len(t, queue.buffer, 0)
}
