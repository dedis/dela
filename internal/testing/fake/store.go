package fake

import "go.dedis.ch/dela/core/store"

// InMemorySnapshot is a fake implementation of a store snapshot.
//
// - implements store.Snapshot
type InMemorySnapshot struct {
	store.Snapshot

	values    map[string][]byte
	ErrRead   error
	ErrWrite  error
	ErrDelete error
}

// NewSnapshot creates a new empty snapshot.
func NewSnapshot() *InMemorySnapshot {
	return &InMemorySnapshot{
		values: make(map[string][]byte),
	}
}

// NewBadSnapshot creates a new empty snapshot that will always return an error.
func NewBadSnapshot() *InMemorySnapshot {
	return &InMemorySnapshot{
		values:    make(map[string][]byte),
		ErrRead:   fakeErr,
		ErrWrite:  fakeErr,
		ErrDelete: fakeErr,
	}
}

// Get implements store.Snapshot.
func (snap *InMemorySnapshot) Get(key []byte) ([]byte, error) {
	return snap.values[string(key)], snap.ErrRead
}

// Set implements store.Snapshot.
func (snap *InMemorySnapshot) Set(key, value []byte) error {
	snap.values[string(key)] = value

	return snap.ErrWrite
}

// Delete implements store.Snapshot.
func (snap *InMemorySnapshot) Delete(key []byte) error {
	delete(snap.values, string(key))

	return snap.ErrDelete
}
