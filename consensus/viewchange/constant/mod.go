package constant

import (
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
)

// ViewChange is a naive implementation of the view change that will
// simply keep the same leader all the time.
//
// - implements viewchange.ViewChange
type ViewChange struct {
	addr mino.Address
}

// NewViewChange returns a new instance of a view change.
func NewViewChange(addr mino.Address) ViewChange {
	return ViewChange{
		addr: addr,
	}
}

// Wait implements viewchange.ViewChange. It returns true if the first player of
// the authority is the current participant.
func (vc ViewChange) Wait(p consensus.Proposal, a crypto.CollectiveAuthority) (uint32, bool) {
	leader := a.AddressIterator().GetNext()

	if !leader.Equal(vc.addr) {
		return 0, false
	}

	return 0, true
}

// Verify implements viewchange.ViewChange. It always return 0 as the leader.
func (vc ViewChange) Verify(consensus.Proposal, crypto.CollectiveAuthority) uint32 {
	return 0
}
