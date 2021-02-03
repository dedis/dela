package simple

import (
	"fmt"
	"sync"

	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/crypto/bls"
)

func ExampleService_GetNonce() {

	exec := native.NewExecution()

	srvc := NewService(exec, signed.NewTransactionFactory())

	signer := bls.NewSigner()
	store := newStore()

	nonce, err := srvc.GetNonce(store, signer.GetPublicKey())
	if err != nil {
		panic("cannot get nonce: " + err.Error())
	}

	fmt.Println(nonce)

	// Output: 0
}

func ExampleService_Validate() {

	exec := native.NewExecution()
	exec.Set("example", exampleContract{})

	srvc := NewService(exec, signed.NewTransactionFactory())

	signer := bls.NewSigner()
	arg := signed.WithArg(native.ContractArg, []byte("example"))

	txA, err := signed.NewTransaction(0, signer.GetPublicKey(), arg)
	if err != nil {
		panic("cannot create transaction A: " + err.Error())
	}
	txB, err := signed.NewTransaction(1, signer.GetPublicKey(), arg)
	if err != nil {
		panic("cannot create transaction B: " + err.Error())
	}
	txC, err := signed.NewTransaction(3, signer.GetPublicKey(), arg)
	if err != nil {
		panic("cannot create transaction C: " + err.Error())
	}

	store := newStore()

	res, err := srvc.Validate(store, []txn.Transaction{txA, txB, txC})
	if err != nil {
		panic("validation failed: " + err.Error())
	}

	for _, txRes := range res.GetTransactionResults() {
		accepted, reason := txRes.GetStatus()

		if accepted {
			fmt.Println("accepted")
		} else {
			fmt.Println("refused", reason)
		}
	}

	// Output: accepted
	// accepted
	// refused nonce is invalid, expected 2, got 3
}

// exampleContract is an example of a contract doing nothing.
//
// - implements baremetal.Contract
type exampleContract struct{}

// Execute implements baremetal.Contract
func (exampleContract) Execute(store.Snapshot, execution.Step) error {
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
