package viewchange

import (
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
)

// ConstantViewChange is a naive implementation of the view change that will
// simply keep the same leader all the time and only allow a leader to propose a
// block.
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

// Wait implements viewchange.ViewChange. It returns an error if the address
// does not match the leader of the previous block. The implementation of the
// returned players is preserved.
func (vc ConstantViewChange) Wait(p consensus.Proposal, a crypto.CollectiveAuthority) (int, bool) {
	leader := a.AddressIterator().GetNext()

	if !leader.Equal(vc.addr) {
		return 0, false
	}

	return 0, true
}

// Verify implements viewchange.ViewChange. It returns an error if the first
// player of the block does not match the address.
func (vc ConstantViewChange) Verify(consensus.Proposal, crypto.CollectiveAuthority) int {
	return 0
}
