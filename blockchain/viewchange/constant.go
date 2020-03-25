package viewchange

import (
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// ConstantViewChange is a naive implementation of the view change that will
// simply keep the same leader all the time and only allow a leader to propose a
// block.
//
// - implements viewchange.ViewChange
type ConstantViewChange struct {
	addr mino.Address
	bc   blockchain.Blockchain
}

// NewConstant returns a new instance of a view change.
func NewConstant(addr mino.Address, bc blockchain.Blockchain) ConstantViewChange {
	return ConstantViewChange{
		addr: addr,
		bc:   bc,
	}
}

// Wait implements viewchange.ViewChange. It returns an error if the address
// does not match the leader of the previous block. The implementation of the
// returned players is preserved.
func (vc ConstantViewChange) Wait(block blockchain.Block) (mino.Players, error) {
	latest, err := vc.bc.GetBlock()
	if err != nil {
		return nil, xerrors.Errorf("couldn't read latest block: %v", err)
	}

	if latest.GetPlayers().Len() == 0 {
		return nil, xerrors.New("players is empty")
	}

	leader := latest.GetPlayers().AddressIterator().GetNext()

	if !leader.Equal(vc.addr) {
		return nil, xerrors.Errorf("mismatching leader: %v != %v", leader, vc.addr)
	}

	return block.GetPlayers(), nil
}

func getLeader(block blockchain.Block) mino.Address {
	return block.GetPlayers().AddressIterator().GetNext()
}

// Verify implements viewchange.ViewChange. It returns an error if the first
// player of the block does not match the address.
func (vc ConstantViewChange) Verify(block blockchain.Block) error {
	latest, err := vc.bc.GetBlock()
	if err != nil {
		return xerrors.Errorf("couldn't read latest block: %v", err)
	}

	newLeader := getLeader(block)
	oldLeader := getLeader(latest)

	if !newLeader.Equal(oldLeader) {
		return xerrors.Errorf("mismatching leader: %v != %v", newLeader, oldLeader)
	}

	return nil
}
