package viewchange

import (
	"go.dedis.ch/fabric/blockchain"
)

// ViewChange provides primitives to verify if a participant is allowed to
// propose a block as the leader. It is also responsible for verifying the
// integrity of the players of the chain.
type ViewChange interface {
	Wait(block blockchain.Block) error
	Verify(block blockchain.Block) error
}
