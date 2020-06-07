package constant

import (
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

// ViewChange is a naive implementation of the view change that will
// simply keep the same leader all the time.
//
// - implements viewchange.ViewChange
type ViewChange struct {
	me        mino.Address
	authority viewchange.Authority
}

// NewViewChange returns a new instance of a view change.
func NewViewChange(addr mino.Address, authority crypto.CollectiveAuthority) ViewChange {
	return ViewChange{
		me:        addr,
		authority: roster.New(authority),
	}
}

// GetAuthority implements viewchange.ViewChange.
func (vc ViewChange) GetAuthority() (viewchange.Authority, error) {
	return vc.authority, nil
}

// Wait implements viewchange.ViewChange. It returns true if the first player of
// the authority is the current participant.
func (vc ViewChange) Wait() bool {
	leader := vc.authority.AddressIterator().GetNext()

	if leader.Equal(vc.me) {
		return true
	}

	return false
}

// Verify implements viewchange.ViewChange. It always return 0 as the leader.
func (vc ViewChange) Verify(from mino.Address) (viewchange.Authority, viewchange.Authority, error) {

	iter := vc.authority.AddressIterator()
	if !iter.HasNext() || !iter.GetNext().Equal(from) {
		return nil, nil, xerrors.Errorf("%v is not the leader", from)
	}

	return vc.authority, vc.authority, nil
}
