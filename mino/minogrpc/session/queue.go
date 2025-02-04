// This file contains an implementation of a non-blocking queue for messages.
//
// Documentation Last Review: 07.10.2020
//

package session

import (
	"go.dedis.ch/dela/mino/router"
	"golang.org/x/xerrors"
	"math"
	"sync"
)

// maximum capacity of the buffer is: (2^limitExponent) * initialCapacity
const initialCapacity = 10000
const limitExponent = 14

// Queue is an interface to queue messages.
type Queue interface {
	Channel() <-chan router.Packet
	Push(router.Packet) error
}

// NonBlockingQueue is an implementation of a queue that makes sure pushing a
// message will never hang. The queue will fill a buffer if the channel is not
// drained and will drop messages when the limit is reached.
//
// - implements session.Queue
type NonBlockingQueue struct {
	sync.Mutex
	working sync.WaitGroup
	buffer  []router.Packet
	cap     float64
	limit   float64
	running bool
	ch      chan router.Packet
}

func newNonBlockingQueue() *NonBlockingQueue {
	return &NonBlockingQueue{
		ch:    make(chan router.Packet, 1),
		cap:   initialCapacity,
		limit: limitExponent,
	}
}

// Channel implements session.Queue. It returns a channel that will be populated
// with incoming messages. The queue uses a buffer when the channel is busy
// therefore this channel should listened to as much as possible to drain the
// messages. At some point when the size of the buffer reaches a limit, messages
// will be dropped.
func (q *NonBlockingQueue) Channel() <-chan router.Packet {
	return q.ch
}

// Push implements session.Queue. It appends the message to the queue without
// blocking. The message is dropped if the queue is at maximum capacity by
// returning an error.
func (q *NonBlockingQueue) Push(msg router.Packet) error {
	select {
	case q.ch <- msg:
		// Message went through !
	default:
		q.Lock()

		if len(q.buffer) == cap(q.buffer) {
			if !q.replaceBuffer() {
				q.Unlock()
				return xerrors.New("queue is at maximum capacity")
			}
		}

		q.buffer = append(q.buffer, msg)

		if !q.running {
			q.running = true
			go q.pushAndWait()
		}
		q.Unlock()
	}

	return nil
}

func (q *NonBlockingQueue) pushAndWait() {
	q.working.Add(1)
	defer q.working.Done()

	for {
		q.Lock()
		if len(q.buffer) == 0 {
			q.running = false
			q.Unlock()
			return
		}

		msg := q.buffer[0]
		q.buffer = q.buffer[1:]

		q.Unlock()

		// Wait for the channel to be available to writings.
		q.ch <- msg
	}
}

func (q *NonBlockingQueue) replaceBuffer() bool {
	exp := float64(0)
	if cap(q.buffer) > 0 {
		exp = math.Floor(math.Log2(float64(cap(q.buffer))/q.cap)) + 1
	}

	if exp > q.limit {
		return false
	}

	newSize := int(math.Pow(2, exp) * q.cap)

	buffer := make([]router.Packet, len(q.buffer), newSize)
	copy(buffer, q.buffer)
	q.buffer = buffer

	return true
}
