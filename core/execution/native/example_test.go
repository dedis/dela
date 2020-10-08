package native

import (
	"encoding/binary"
	"fmt"
	"sync"

	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/crypto/bls"
)

func ExampleService_Execute() {
	srvc := NewExecution()
	srvc.Set("example", exampleContract{})

	store := newStore()
	signer := bls.NewSigner()

	increment := make([]byte, 8)
	binary.LittleEndian.PutUint64(increment, 5)

	opts := []signed.TransactionOption{
		signed.WithArg("increment", increment),
		signed.WithArg(ContractArg, []byte("example")),
	}

	tx, err := signed.NewTransaction(0, signer.GetPublicKey(), opts...)
	if err != nil {
		panic("failed to create transaction: " + err.Error())
	}

	step := execution.Step{
		Current: tx,
	}

	for i := 0; i < 2; i++ {
		res, err := srvc.Execute(store, step)
		if err != nil {
			panic("failed to execute: " + err.Error())
		}

		if res.Accepted {
			fmt.Println("accepted")
		}
	}

	value, err := store.Get([]byte("counter"))
	if err != nil {
		panic("store failed: " + err.Error())
	}

	fmt.Println(binary.LittleEndian.Uint64(value))

	// Output: accepted
	// accepted
	// 10
}

// exampleContract is an example contract that reads a counter value in the
// store and increase it with the increment in the transaction.
//
// - implements native.Contract
type exampleContract struct{}

// Execute implements native.Contract. It increases the counter with the
// increment in the transaction.
func (exampleContract) Execute(store store.Snapshot, step execution.Step) error {
	value, err := store.Get([]byte("counter"))
	if err != nil {
		return err
	}

	counter := uint64(0)
	if len(value) == 8 {
		counter = binary.LittleEndian.Uint64(value)
	}

	incr := binary.LittleEndian.Uint64(step.Current.GetArg("increment"))

	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, counter+incr)

	err = store.Set([]byte("counter"), buffer)
	if err != nil {
		return err
	}

	return nil
}

// inMemoryStore in a simple implementation of a store using an in-memory
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
