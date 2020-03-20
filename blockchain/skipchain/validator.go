package skipchain

import (
	"bytes"
	"sync"

	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/encoding"
	"golang.org/x/xerrors"
)

// blockValidator is a validator for incoming protobuf messages to decode them
// into a skipblock and for commit queued blocks when appropriate.
//
// - implements consensus.Validator
type blockValidator struct {
	*Skipchain

	validator blockchain.Validator
	queue     *blockQueue
	watcher   blockchain.Observable
}

func newBlockValidator(
	s *Skipchain,
	v blockchain.Validator,
	w blockchain.Observable,
) *blockValidator {
	return &blockValidator{
		Skipchain: s,
		validator: v,
		watcher:   w,
		queue: &blockQueue{
			buffer: make(map[Digest]SkipBlock),
		},
	}
}

// Validate implements consensus.Validator. It decodes the message into a block
// and validates its integrity. It returns the block if it is correct, otherwise
// the error.
func (v *blockValidator) Validate(pb proto.Message) (consensus.Proposal, error) {
	factory := v.GetBlockFactory().(blockFactory)

	block, err := factory.decodeBlock(pb)
	if err != nil {
		return nil, encoding.NewDecodingError("block", err)
	}

	genesis, err := v.db.Read(0)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read genesis block: %v", err)
	}

	if !bytes.Equal(genesis.GetHash(), block.GenesisID.Bytes()) {
		return nil, xerrors.Errorf("mismatch genesis hash '%v' != '%v'",
			genesis.hash, block.GenesisID)
	}

	err = v.validator.Validate(block.Payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't validate the payload: %v", err)
	}

	v.queue.Add(block)

	return block, nil
}

// Commit implements consensus.Validator. It commits the block that matches the
// identifier if it is present.
func (v *blockValidator) Commit(id []byte) error {
	digest := Digest{}
	copy(digest[:], id)

	block, ok := v.queue.Get(digest)
	if !ok {
		return xerrors.Errorf("couldn't find block '%v'", digest)
	}

	err := v.db.Atomic(func(ops Queries) error {
		err := ops.Write(block)
		if err != nil {
			return xerrors.Errorf("couldn't persist the block: %v", err)
		}

		err = v.validator.Commit(block.Payload)
		if err != nil {
			// If the upper layer fails to commit to the block, it won't be
			// written so that the node keeps a stable state.
			return xerrors.Errorf("couldn't commit the payload: %v", err)
		}

		return nil
	})

	if err != nil {
		return xerrors.Errorf("transaction aborted: %v", err)
	}

	// Notify every observer that we committed to a new block. This is blocking
	// to allow atomic operations.
	v.watcher.Notify(block)

	fabric.Logger.Trace().Msgf("commit to block %v", block.hash)
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
