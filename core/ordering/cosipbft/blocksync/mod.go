package blocksync

import (
	"context"

	"go.dedis.ch/dela/mino"
)

// Config is the configuration to change the behaviour of the synchronization.
type Config struct {
	MinSoft int
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
