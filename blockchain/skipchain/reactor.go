package skipchain

import (
	"sync"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// reactor is following the reactor design pattern to implement a consensus
// reactor. It reacts to event coming from the consensus module and returns the
// expected data.
//
// - implements consensus.Reactor
type reactor struct {
	BlueprintFactory
	*operations

	queue *blockQueue
}

func newReactor(ops *operations) *reactor {
	return &reactor{
		operations: ops,
		queue: &blockQueue{
			buffer: make(map[Digest]SkipBlock),
		},
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
func (v *reactor) InvokeValidate(addr mino.Address, pb serde.Message) ([]byte, error) {
	blueprint, ok := pb.(Blueprint)
	if !ok {
		return nil, xerrors.New("invalid blueprint type")
	}

	// It makes sure that we know the whole chain up to the previous proposal.
	err := v.catchUp(blueprint.index, addr)
	if err != nil {
		return nil, xerrors.Errorf("couldn't catch up: %v", err)
	}

	genesis, err := v.db.Read(0)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read genesis block: %v", err)
	}

	err = v.processor.Validate(blueprint.payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't validate the payload: %v", err)
	}

	block := SkipBlock{
		Index:     blueprint.index,
		GenesisID: genesis.hash,
		BackLink:  blueprint.previous,
		Payload:   blueprint.payload,
	}

	hash, err := block.computeHash(v.blockFactory.hashFactory, v.encoder)
	if err != nil {
		return nil, err
	}

	block.hash = hash

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

	digest := Digest{}
	copy(digest[:], id)

	block, ok := v.queue.Get(digest)
	if !ok {
		return xerrors.Errorf("couldn't find block '%v'", digest)
	}

	err := v.commitBlock(block)
	if err != nil {
		return xerrors.Errorf("couldn't commit block: %v", err)
	}

	v.queue.Clear()

	return nil
}

type blockQueue struct {
	sync.Mutex

	buffer map[Digest]SkipBlock
}

func (q *blockQueue) Get(id Digest) (SkipBlock, bool) {
	q.Lock()
	defer q.Unlock()

	block, ok := q.buffer[id]
	return block, ok
}

func (q *blockQueue) Add(block SkipBlock) {
	q.Lock()
	defer q.Unlock()

	// As the block is indexed by the hash, it does not matter if it overrides
	// an already existing one.
	q.buffer[block.hash] = block
}

func (q *blockQueue) Clear() {
	q.Lock()
	defer q.Unlock()

	q.buffer = make(map[Digest]SkipBlock)
}
