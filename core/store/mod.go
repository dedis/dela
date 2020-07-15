package store

// Readable is the interface for a readable store.
type Readable interface {
	Get(key []byte) ([]byte, error)
}

// Writable is the interface for a writable store.
type Writable interface {
	Set(key []byte, value []byte) error

	Delete(key []byte) error
}

// Snapshot is a state of the store that can be read and write independently. A
// write is applied only to the snapshot reference.
type Snapshot interface {
	Readable
	Writable
}
