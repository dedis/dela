package skipchain

import (
	"bytes"

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
	buffer    SkipBlock
}

// Validate implements consensus.Validator. It decodes the message into a block
// and validate its integrity. It returns the block if it is correct, otherwise
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
		return nil, xerrors.Errorf("mismatch genesis hash %v != %v", genesis.hash, block.GenesisID)
	}

	err = v.validator.Validate(block.Payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't validate the payload: %v", err)
	}

	v.buffer = block

	return block, nil
}

// Commit implements consensus.Validator. It commits the block that matches the
// identifier if it is present.
func (v *blockValidator) Commit(id []byte) error {
	// TODO: multi-block buffer.
	// TODO: atomic operation if commit failed.
	block := v.buffer

	if !bytes.Equal(id, block.GetHash()) {
		return xerrors.Errorf("unknown block %x", id)
	}

	err := v.db.Write(block)
	if err != nil {
		return xerrors.Errorf("couldn't persist the block: %v", err)
	}

	err = v.validator.Commit(block.Payload)
	if err != nil {
		return xerrors.Errorf("couldn't commit the payload: %v", err)
	}

	fabric.Logger.Trace().Msgf("commit to block %v", block.hash)

	return nil
}
