package types

import "sync"

// Queue is the interface to implement to store a block waiting to be committed.
type Queue interface {
	Len() int
	Get(id []byte) (SkipBlock, bool)
	Add(SkipBlock)
	Clear()
}

type simpleQueue struct {
	sync.Mutex

	buffer map[Digest]SkipBlock
}

// NewQueue creates a new empty queue.
func NewQueue() Queue {
	return &simpleQueue{
		buffer: make(map[Digest]SkipBlock),
	}
}

func (q *simpleQueue) Len() int {
	q.Lock()
	defer q.Unlock()

	return len(q.buffer)
}

func (q *simpleQueue) Get(id []byte) (SkipBlock, bool) {
	q.Lock()
	defer q.Unlock()

	key := Digest{}
	copy(key[:], id)

	block, ok := q.buffer[key]
	return block, ok
}

func (q *simpleQueue) Add(block SkipBlock) {
	q.Lock()
	defer q.Unlock()

	// As the block is indexed by the hash, it does not matter if it overrides
	// an already existing one.
	q.buffer[block.hash] = block
}

func (q *simpleQueue) Clear() {
	q.Lock()
	defer q.Unlock()

	q.buffer = make(map[Digest]SkipBlock)
}
