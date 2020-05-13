package skipchain

import (
	"bytes"
	"sync"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// blockValidator is a validator for incoming protobuf messages to decode them
// into a skipblock and for commit queued blocks when appropriate.
//
// - implements consensus.Validator
type blockValidator struct {
	*operations
	queue *blockQueue

	// When a catch is in progress, validations and commits should be delayed to
	// let the system catches up to the latest known state.
	catchUpLock sync.Mutex
}

func newBlockValidator(ops *operations) *blockValidator {
	return &blockValidator{
		operations: ops,
		queue: &blockQueue{
			buffer: make(map[Digest]SkipBlock),
		},
	}
}

// Validate implements consensus.Validator. It decodes the message into a block
// and validates its integrity. It returns the block if it is correct, otherwise
// the error.
func (v *blockValidator) Validate(addr mino.Address,
	pb proto.Message) (consensus.Proposal, error) {

	block, err := v.blockFactory.decodeBlock(pb)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode block: %v", err)
	}

	v.catchUpLock.Lock()
	once := sync.Once{}
	defer once.Do(func() { v.catchUpLock.Unlock() })

	// It makes sure that we know the whole chain up to the previous proposal.
	err = v.catchUp(block, addr)
	if err != nil {
		return nil, xerrors.Errorf("couldn't catch up: %v", err)
	}

	// Override the defer to free the lock as soon as possible.
	once.Do(func() { v.catchUpLock.Unlock() })

	genesis, err := v.db.Read(0)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read genesis block: %v", err)
	}

	if !bytes.Equal(genesis.GetHash(), block.GenesisID.Bytes()) {
		return nil, xerrors.Errorf("mismatch genesis hash '%v' != '%v'",
			genesis.hash, block.GenesisID)
	}

	err = v.processor.Validate(block.Index, block.Payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't validate the payload: %v", err)
	}

	v.queue.Add(block)

	return block, nil
}

// Commit implements consensus.Validator. It commits the block that matches the
// identifier if it is present.
func (v *blockValidator) Commit(id []byte) error {
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
