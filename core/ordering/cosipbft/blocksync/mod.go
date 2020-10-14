// Package blocksync defines a block synchronizer for the ordering service.
//
// The package also implements a default synchronizer that will send an
// announcement with the latest known block, and share the chain to the nodes
// that have fallen behind.
//
// Documentation Last Review: 13.10.2020
//
package blocksync

import (
	"context"

	"go.dedis.ch/dela/mino"
)

// Config is the configuration to change the behaviour of the synchronization.
type Config struct {
	// MinSoft is the number of participants that have soft-synchronized,
	// meaning they know the latest index of the leader.
	MinSoft int

	// MinHard is the number of participants that have hard-synchronized,
	// meaning they have the latest block stored.
	MinHard int
}

// Synchronizer is an interface to synchronize a leader with the participants.
type Synchronizer interface {
	// GetLatest returns the latest known synchronization update. It can be used
	// to wait for a complete chain update as this index has been proven to
	// exist.
	GetLatest() uint64

	// Sync sends a synchronization message to all the participants in order to
	// announce the current state of the chain.
	Sync(ctx context.Context, players mino.Players, cfg Config) error
}
