package skipchain

import "golang.org/x/xerrors"

// Database is an interface that provides the primitives to read and write
// blocks to a storage.
type Database interface {
	Write(block SkipBlock) error
	Read(index int64) (SkipBlock, error)
	ReadLast() (SkipBlock, error)
	ReadChain() (Chain, error)
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
		return xerrors.New("missing intermediate blocks")
	}

	return nil
}

func (db *InMemoryDatabase) Read(index int64) (SkipBlock, error) {
	if index < int64(len(db.blocks)) {
		return db.blocks[index], nil
	}

	return SkipBlock{}, xerrors.New("block not found")
}

// ReadLast reads the last known block of the chain.
func (db *InMemoryDatabase) ReadLast() (SkipBlock, error) {
	if len(db.blocks) == 0 {
		return SkipBlock{}, xerrors.New("missing genesis block")
	}

	return db.blocks[len(db.blocks)-1], nil
}

// ReadChain returns the list of blocks available.
func (db *InMemoryDatabase) ReadChain() (Chain, error) {
	return db.blocks, nil
}
