package skipchain

import (
	"errors"
)

// Database is an interface that provides the primitives to read and write
// blocks to a storage.
type Database interface {
	Write(block SkipBlock) error
	Read(index int64) (SkipBlock, error)
	ReadLast() (SkipBlock, error)
}

// InMemoryDatabase is an implementation of the database interface that is
// an in-memory storage.
type InMemoryDatabase struct {
	blocks []SkipBlock
}

// NewInMemoryDatabase creates a new in-memory storage for blocks.
func NewInMemoryDatabase() *InMemoryDatabase {
	return &InMemoryDatabase{
		blocks: make([]SkipBlock, 1),
	}
}

func (db *InMemoryDatabase) Write(block SkipBlock) error {
	if uint64(len(db.blocks)) == block.Index {
		db.blocks = append(db.blocks, block)
	} else if uint64(len(db.blocks)) > block.Index {
		db.blocks[block.Index] = block
	} else {
		return errors.New("missing intermediate blocks")
	}

	return nil
}

func (db *InMemoryDatabase) Read(index int64) (SkipBlock, error) {
	if index < int64(len(db.blocks)) {
		return db.blocks[index], nil
	}

	return SkipBlock{}, errors.New("block not found")
}

// ReadLast reads the last known block of the chain.
func (db *InMemoryDatabase) ReadLast() (SkipBlock, error) {
	if len(db.blocks) == 0 {
		return SkipBlock{}, errors.New("missing genesis block")
	}

	return db.blocks[len(db.blocks)-1], nil
}
