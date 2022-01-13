package tcp

import (
	"encoding/binary"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/txn"
)

func TestService_Execute(t *testing.T) {
	store := newStore()

	initialCounter := uint64(1234)

	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, initialCounter)
	store.Set(storeKey[:], buffer)

	srvc := NewExecution()

	step := execution.Step{
		Current: fakeTx{address: "localhost:6789"},
	}

	res, err := srvc.Execute(store, step)
	require.NoError(t, err)
	require.Equal(t, execution.Result{Accepted: true}, res)

	buffer, err = store.Get(storeKey[:])
	newCounter := binary.LittleEndian.Uint64(buffer)

	require.Equal(t, initialCounter+1, newCounter)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeTx struct {
	txn.Transaction
	address  string
	contract string
}

func (ftx fakeTx) GetArg(key string) []byte {
	return []byte(ftx.address)
}

// inMemoryStore is a simple implementation of a store using an in-memory
// map.
//
// - implements store.Snapshot
type inMemoryStore struct {
	sync.Mutex

	entries map[string][]byte
}

func newStore() *inMemoryStore {
	return &inMemoryStore{
		entries: make(map[string][]byte),
	}
}

// Get implements store.Readable. It returns the value associated to the key.
func (s *inMemoryStore) Get(key []byte) ([]byte, error) {
	s.Lock()
	defer s.Unlock()

	return s.entries[string(key)], nil
}

// Set implements store.Writable. It sets the value for the key.
func (s *inMemoryStore) Set(key, value []byte) error {
	s.Lock()
	s.entries[string(key)] = value
	s.Unlock()

	return nil
}

// Delete implements store.Writable. It deletes the key from the store.
func (s *inMemoryStore) Delete(key []byte) error {
	s.Lock()
	delete(s.entries, string(key))
	s.Unlock()

	return nil
}
