package darc

import (
	"fmt"
	"sync"

	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/serde/json"
)

func ExampleService_Grant_alone() {
	srvc := NewService(json.NewContext())

	store := newStore()

	alice := bls.NewSigner()
	bob := bls.NewSigner()

	credential := access.NewContractCreds([]byte{1}, "example", "hello")

	err := srvc.Grant(store, credential, alice.GetPublicKey())
	if err != nil {
		panic("failed to grant alice: " + err.Error())
	}

	err = srvc.Match(store, credential, alice.GetPublicKey())
	if err != nil {
		panic("alice has no access: " + err.Error())
	} else {
		fmt.Println("Alice has the access")
	}

	err = srvc.Match(store, credential, bob.GetPublicKey())
	if err != nil {
		fmt.Println("Bob has no access")
	}

	// Output: Alice has the access
	// Bob has no access
}

func ExampleService_Grant_group() {
	srvc := NewService(json.NewContext())

	store := newStore()

	alice := bls.NewSigner()
	bob := bls.NewSigner()

	credential := access.NewContractCreds([]byte{1}, "example", "hello")

	err := srvc.Grant(store, credential, alice.GetPublicKey(), bob.GetPublicKey())
	if err != nil {
		panic("failed to grant alice: " + err.Error())
	}

	err = srvc.Match(store, credential, alice.GetPublicKey(), bob.GetPublicKey())
	if err != nil {
		panic("alice and bob have no access: " + err.Error())
	} else {
		fmt.Println("[Alice, Bob] have the access")
	}

	err = srvc.Match(store, credential, alice.GetPublicKey())
	if err != nil {
		fmt.Println("Alice alone has no access")
	}

	// Output: [Alice, Bob] have the access
	// Alice alone has no access
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
