package blocksync

import (
	"context"

	"go.dedis.ch/dela/mino"
)

type Event struct {
	Soft   int
	Hard   int
	Errors []error
}

type Synchronizer interface {
	// GetLatest returns the latest known synchronization update. It can be used
	// to wait for a complete chain update as this index has been proven to
	// exist.
	GetLatest() uint64

	// Sync sends a synchronization message to all the participants in order to
	// announce the current state of the chain.
	Sync(ctx context.Context, players mino.Players) (<-chan Event, error)
}
