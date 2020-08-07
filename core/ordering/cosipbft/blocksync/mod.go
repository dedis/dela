package blocksync

import (
	"context"

	"go.dedis.ch/dela/mino"
)

// Event is an event triggered to update on the synchronization status.
type Event struct {
	// Soft is the number of participants that have soft-synchronized, meaning
	// they know the latest index of the leader.
	Soft int

	// Hard is the number of participants that have hard-synchronized, meaning
	// they have the latest block stored.
	Hard int

	// Errors contains any error that happened during the synchronization.
	Errors []error
}

// Synchronizer is an interface to synchronize a leader with the participants.
type Synchronizer interface {
	// GetLatest returns the latest known synchronization update. It can be used
	// to wait for a complete chain update as this index has been proven to
	// exist.
	GetLatest() uint64

	// Sync sends a synchronization message to all the participants in order to
	// announce the current state of the chain.
	Sync(ctx context.Context, players mino.Players) <-chan Event
}
