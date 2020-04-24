package viewchange

import (
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
)

// ConstantViewChange is a naive implementation of the view change that will
// simply keep the same leader all the time.
//
// - implements viewchange.ViewChange
type ConstantViewChange struct {
	addr mino.Address
}

// NewConstant returns a new instance of a view change.
func NewConstant(addr mino.Address) ConstantViewChange {
	return ConstantViewChange{
		addr: addr,
	}
}

// Wait implements viewchange.ViewChange. It true if the first player of the
// authority is the current participant.
func (vc ConstantViewChange) Wait(p consensus.Proposal, a crypto.CollectiveAuthority) (int, bool) {
	leader := a.AddressIterator().GetNext()

	if !leader.Equal(vc.addr) {
		return 0, false
	}

	return 0, true
}

// Verify implements viewchange.ViewChange. It always return 0 that means no
// rotation.
func (vc ConstantViewChange) Verify(consensus.Proposal, crypto.CollectiveAuthority) int {
	return 0
}
