package prefixed

import (
	"encoding/binary"

	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/crypto"
)

type readable struct {
	store.Readable
	prefix []byte
}

type writable struct {
	store.Writable
	prefix []byte
}

type snapshot struct {
	*writable
	*readable
}

// NewSnapshot creates a new prefixed Snapshot.
func NewSnapshot(prefix string, snap store.Snapshot) store.Snapshot {
	p := []byte(prefix)
	return &snapshot{
		&writable{snap, p},
		&readable{snap, p},
	}
}

// NewReadable creates a new prefixed Readable.
func NewReadable(prefix string, r store.Readable) store.Readable {
	p := []byte(prefix)
	return &readable{r, p}
}

// Get implements store.Readable
// It takes a key as input and returns a value or error status
func (s *readable) Get(key []byte) ([]byte, error) {

	k := NewPrefixedKey(s.prefix, key)
	return s.Readable.Get(k)
}

// Set implements store.Writable
// It takes a key and value as input, and returns an error status
func (s *writable) Set(key []byte, value []byte) error {
	k := NewPrefixedKey(s.prefix, key)
	return s.Writable.Set(k, value)
}

// Delete implements store.Writable
// It takes a key as input and returns an error status
func (s *writable) Delete(key []byte) error {
	k := NewPrefixedKey(s.prefix, key)
	return s.Writable.Delete(k)
}

// NewPrefixedKey is exported because it is used in integration tests.
// It creates a 256bit (hashed) key from a prefix and a base key.
func NewPrefixedKey(prefix, key []byte) []byte {
	h := crypto.NewHashFactory(crypto.Sha256).New()

	length := []byte{0, 0}
	binary.LittleEndian.PutUint16(length, uint16(len(prefix)))

	h.Write(length)
	h.Write(prefix)

	length = []byte{0, 0}
	binary.LittleEndian.PutUint16(length, uint16(len(key)))

	h.Write(length)
	h.Write(key)

	return h.Sum(nil)
}
