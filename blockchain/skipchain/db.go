package skipchain

import (
	fmt "fmt"
	"sync"

	"golang.org/x/xerrors"
)

// Queries is an interface to provide high-level queries to store and read
// blocks.
type Queries interface {
	Write(block SkipBlock) error
	Read(index int64) (SkipBlock, error)
	ReadLast() (SkipBlock, error)
}

// Database is an interface that provides the primitives to read and write
// blocks to a storage.
type Database interface {
	Queries

	// Atomic allows the execution of atomic operations. If the callback returns
	// any error, the transaction will be aborted.
	Atomic(func(ops Queries) error) error
}

// NoBlockError is an error returned when the block is not found. It can be used
// in comparison as it complies with the xerrors.Is requirement.
type NoBlockError struct {
	index int64
}

// NewNoBlockError returns a new instance of the error.
func NewNoBlockError(index int64) NoBlockError {
	return NoBlockError{index: index}
}

func (err NoBlockError) Error() string {
	return fmt.Sprintf("block at index %d not found", err.index)
}

// Is returns true when both errors are equal, otherwise it returns false.
func (err NoBlockError) Is(other error) bool {
	otherErr, ok := other.(NoBlockError)
	return ok && otherErr.index == err.index
}

// InMemoryDatabase is an implementation of the database interface that is
// an in-memory storage.
//
// - implements skipchain.Database
type InMemoryDatabase struct {
	sync.Mutex
	blocks []SkipBlock
}

// NewInMemoryDatabase creates a new in-memory storage for blocks.
func NewInMemoryDatabase() *InMemoryDatabase {
	return &InMemoryDatabase{
		blocks: make([]SkipBlock, 0),
	}
}

// Write implements skipchain.Database. It writes the block to the storage.
func (db *InMemoryDatabase) Write(block SkipBlock) error {
	db.Lock()
	defer db.Unlock()

	if uint64(len(db.blocks)) == block.Index {
		db.blocks = append(db.blocks, block)
	} else if block.Index < uint64(len(db.blocks)) {
		db.blocks[block.Index] = block
	} else {
		return xerrors.Errorf("missing intermediate blocks for index %d", block.Index)
	}

	return nil
}

// Read implements skipchain.Database. It returns the block at the given index
// if it exists, otherwise an error.
func (db *InMemoryDatabase) Read(index int64) (SkipBlock, error) {
	db.Lock()
	defer db.Unlock()

	if index < int64(len(db.blocks)) {
		return db.blocks[index], nil
	}

	return SkipBlock{}, NewNoBlockError(index)
}

// ReadLast implements skipchain.Database. It reads the last known block of the
// chain.
func (db *InMemoryDatabase) ReadLast() (SkipBlock, error) {
	db.Lock()
	defer db.Unlock()

	if len(db.blocks) == 0 {
		return SkipBlock{}, xerrors.New("database is empty")
	}

	return db.blocks[len(db.blocks)-1], nil
}

// Atomic implements skipchain.Database. It executes the transaction so that any
// error returned will revert any previous operations.
func (db *InMemoryDatabase) Atomic(tx func(Queries) error) error {
	db.Lock()
	defer db.Unlock()

	snapshot := db.clone()

	err := tx(snapshot)
	if err != nil {
		return xerrors.Errorf("couldn't execute transaction: %v", err)
	}

	db.blocks = snapshot.blocks

	return nil
}

// clone returns a deep copy of the in-memory database.
func (db *InMemoryDatabase) clone() *InMemoryDatabase {
	blocks := make([]SkipBlock, len(db.blocks))
	copy(blocks, db.blocks)

	return &InMemoryDatabase{
		blocks: blocks,
	}
}
