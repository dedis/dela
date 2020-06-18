package types

import "sync"

type Queue interface {
	Len() int
	Get(id []byte) (SkipBlock, bool)
	Add(SkipBlock)
	Clear()
}

type blockQueue struct {
	sync.Mutex

	buffer map[Digest]SkipBlock
}

func NewQueue() Queue {
	return &blockQueue{
		buffer: make(map[Digest]SkipBlock),
	}
}

func (q *blockQueue) Len() int {
	q.Lock()
	defer q.Unlock()

	return len(q.buffer)
}

func (q *blockQueue) Get(id []byte) (SkipBlock, bool) {
	q.Lock()
	defer q.Unlock()

	key := Digest{}
	copy(key[:], id)

	block, ok := q.buffer[key]
	return block, ok
}

func (q *blockQueue) Add(block SkipBlock) {
	q.Lock()
	defer q.Unlock()

	// As the block is indexed by the hash, it does not matter if it overrides
	// an already existing one.
	q.buffer[block.hash] = block
}

func (q *blockQueue) Clear() {
	q.Lock()
	defer q.Unlock()

	q.buffer = make(map[Digest]SkipBlock)
}
