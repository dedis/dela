// Package ordering defines the interface of the ordering service. The
// high-level purpose of this service is to order the transactions from the
// pool.
//
// Depending on the implementation, the service can be composed of multiple
// sub-components. For instance, an ordering service using CoSiPBFT will need to
// elect a leader every round but one running PoW will only do an ordering
// locally and creates a block with the proof of work.
package ordering

import "context"

// Proof contains the value of a specific key.
type Proof interface {
	// GetKey returns the key of the proof.
	GetKey() []byte

	// GetValue returns the value of the key.
	GetValue() []byte
}

// Event describes the current state of the service after an update.
type Event struct {
	Index uint64
}

// Service is the interface of an ordering service. It provides the primitives
// to order transactions from a pool.
type Service interface {
	// Listen opens the endpoints of the service so that other participants can
	// contact the node.
	Listen() error

	// Close closes the service.
	Close() error

	// GetProof must return a proof of the value at the provided key.
	GetProof(key []byte) (Proof, error)

	// Watch returns channel populated with events when transactions are
	// accepted.
	Watch(ctx context.Context) <-chan Event
}
