// Package ordering defines the interface of the ordering service. The
// high-level purpose of this service is to order the transactions from the
// pool.
//
// Depending on the implementation, the service can be composed of multiple
// sub-components. For instance, an ordering service using CoSiPBFT will need to
// elect a leader every round but one running PoW will only do an ordering
// locally and creates a block with the proof of work.
package ordering

import (
	"context"

	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/validation"
)

// Proof contains the value of a specific key.
type Proof interface {
	// GetKey returns the key of the proof.
	GetKey() []byte

	// GetValue returns the value of the key.
	GetValue() []byte
}

// Event describes the current state of the service after an update.
type Event struct {
	Index        uint64
	Transactions []validation.TransactionResult
}

// Service is the interface of an ordering service. It provides the primitives
// to order transactions from a pool.
type Service interface {
	// GetProof must return a proof of the value at the provided key.
	GetProof(key []byte) (Proof, error)

	// GetStore returns the store used by the service.
	GetStore() store.Readable

	// Watch returns a channel populated with events when transactions are
	// accepted.
	Watch(ctx context.Context) <-chan Event

	// Close closes the service and cleans the resources.
	Close() error
}
