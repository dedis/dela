// Package rotating implements a rotating view change based on the proposal
// index so that the leader changes after every round.
package rotating

import (
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
)

// ViewChange is an implementation of the view change interface that will rotate
// the leader based on the index of the proposal.
//
// - implements viewchange.ViewChange
type ViewChange struct {
	addr mino.Address
}

// NewViewChange returns a new instance of the rotating view change.
func NewViewChange(addr mino.Address) ViewChange {
	return ViewChange{addr: addr}
}

// Wait implements viewchange.ViewChange. It calculates the next leader index
// from the authority length and returns true if the local participant is the
// leader.
func (vc ViewChange) Wait(prop consensus.Proposal, authority crypto.CollectiveAuthority) (uint32, bool) {
	leader := int(prop.GetIndex()) % authority.Len()

	iter := authority.AddressIterator()
	iter.Seek(leader)
	if iter.HasNext() && iter.GetNext().Equal(vc.addr) {
		return uint32(leader), true
	}

	return uint32(leader), false
}

// Verify implements viewchange.ViewChange. It calculates the leader for the
// given proposal from the authority length.
func (vc ViewChange) Verify(prop consensus.Proposal, authority crypto.CollectiveAuthority) uint32 {
	leader := int(prop.GetIndex()) % authority.Len()

	return uint32(leader)
}
