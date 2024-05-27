// Package fastsync defines a block synchronizer for the ordering service.
//
// The block synchronizer is to be called in two situations:
// - if a node is starting up, to make sure it's up-to-date with other nodes
// - if a node receives a request for a block it doesn't hold the parent of
//
// To make it really simple, the node sends a catchup request parallel to
// f+1 random nodes.
// As long as there are enough honest nodes, this will allow the block to
// catch up to the latest block.
// One optimization would be to send the requests serially, waiting for the
// reply before going on.
// But this would involve timeouts and would take much longer.
// So we suppose the node is not that much behind and thus will not waste too
// much bandwidth.
//
// Possible improvements:
// - make the protocol more efficient in the presence of byzantine nodes:
// The node broadcasts a request indicating which is the last block in storage.
// It receives offers from different nodes, and contacts the n nodes with the
// most recent block, where n must be bigger than the maximum number of
// byzantine nodes.
//
// Documentation Last Review: 22.11.2023
package fastsync

import (
	"context"

	"go.dedis.ch/dela/mino"
)

// Config of the current run of the fastsync.
// For future expansion and to make it similar to blocksync,
// this is held as a struct.
type Config struct {
	// The size at which the message will be split.
	// If the encoding of all blocks is bigger than this value, the
	// message is sent as-is.
	SplitMessageSize uint64
}

// Synchronizer is an interface to synchronize a node with the participants.
type Synchronizer interface {
	// Sync sends a synchronization request message to f+1 random participants,
	// which will return BlockLinks to the latest block.
	Sync(ctx context.Context, players mino.Players, config Config) error
}
