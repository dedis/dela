package skipchain

import (
	"go.dedis.ch/dela/blockchain/skipchain/types"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

// reactor is following the reactor design pattern to implement a consensus
// reactor. It reacts to event coming from the consensus module and returns the
// expected data.
//
// - implements consensus.Reactor
type reactor struct {
	types.BlueprintFactory
	*operations

	queue types.Queue
}

func newReactor(ops *operations) *reactor {
	return &reactor{
		BlueprintFactory: types.NewBlueprintFactory(ops.reactor),
		operations:       ops,
		queue:            types.NewQueue(),
	}
}

// InvokeGenesis implements consensus.Reactor. It returns the hash of the
// genesis block.
func (v *reactor) InvokeGenesis() ([]byte, error) {
	genesis, err := v.db.Read(0)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read genesis block: %v", err)
	}

	return genesis.GetHash(), nil
}

// InvokeValidate implements consensus.Reactor. It decodes the message into a
// block and validates its integrity. It returns the block if it is correct,
// otherwise the error.
func (v *reactor) InvokeValidate(addr mino.Address, pb serdeng.Message) ([]byte, error) {
	blueprint, ok := pb.(types.Blueprint)
	if !ok {
		return nil, xerrors.Errorf("invalid message type '%T'", pb)
	}

	// It makes sure that we know the whole chain up to the previous proposal.
	err := v.catchUp(blueprint.GetIndex(), addr)
	if err != nil {
		return nil, xerrors.Errorf("couldn't catch up: %v", err)
	}

	genesis, err := v.db.Read(0)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read genesis block: %v", err)
	}

	payload, err := v.reactor.InvokeValidate(blueprint.GetData())
	if err != nil {
		return nil, xerrors.Errorf("couldn't validate the payload: %v", err)
	}

	block, err := types.NewSkipBlock(
		types.WithIndex(blueprint.GetIndex()),
		types.WithGenesisID(genesis.GetHash()),
		types.WithBackLink(blueprint.GetPrevious()),
		types.WithPayload(payload))

	if err != nil {
		return nil, xerrors.Errorf("couldn't compute hash: %v", err)
	}

	v.queue.Add(block)

	return block.GetHash(), nil
}

// InvokeCommit implements consensus.Reactor. It commits the block that matches
// the identifier if it is present.
func (v *reactor) InvokeCommit(id []byte) error {
	// To minimize the catch up procedures, the lock is acquired so that it can
	// process a new block before the catch up verifies what is the latest
	// known.
	v.catchUpLock.Lock()
	defer v.catchUpLock.Unlock()

	block, ok := v.queue.Get(id)
	if !ok {
		return xerrors.Errorf("couldn't find block %#x", id)
	}

	err := v.commitBlock(block)
	if err != nil {
		return xerrors.Errorf("couldn't commit block: %v", err)
	}

	v.queue.Clear()

	return nil
}
