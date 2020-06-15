package skipchain

import (
	"encoding/binary"
	"fmt"
	"io"

	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/blockchain/skipchain/json"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// Digest is an alias of a slice of bytes that represents the digest of a block.
type Digest [32]byte

// Bytes returns the slice of bytes representing the digest.
func (d Digest) Bytes() []byte {
	return d[:]
}

// String returns the short string representation of the digest.
func (d Digest) String() string {
	return fmt.Sprintf("%x", d[:])[:16]
}

// SkipBlock is a representation of the data held by a block. It contains the
// information to build a skipchain.
//
// - implements blockchain.Block
// - implements consensus.Proposal
// - implements fmt.Stringer
type SkipBlock struct {
	serde.UnimplementedMessage

	hash Digest

	// Index is the block index since the genesis block.
	Index uint64

	// GenesisID is the hash of the genesis block which represents the chain
	// identifier.
	GenesisID Digest

	// BackLink is the hash of the previous block in the chain.
	BackLink Digest

	// Payload is the data stored in the block. It representation is independant
	// from the skipchain module.
	Payload blockchain.Payload
}

// GetIndex returns the index of the block since the genesis block.
func (b SkipBlock) GetIndex() uint64 {
	return b.Index
}

// GetHash implements blockchain.Block and consensus.Proposal. It returns the
// hash of the block.
func (b SkipBlock) GetHash() []byte {
	return b.hash[:]
}

// GetPayload implements blockchain.Block. It returns the block payload.
func (b SkipBlock) GetPayload() blockchain.Payload {
	return b.Payload
}

// VisitJSON implements serde.Message. It serializes the block in JSON format.
func (b SkipBlock) VisitJSON(ser serde.Serializer) (interface{}, error) {
	payload, err := ser.Serialize(b.Payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize payload: %v", err)
	}

	m := json.SkipBlock{
		Index:     b.Index,
		GenesisID: b.GenesisID.Bytes(),
		Backlink:  b.BackLink.Bytes(),
		Payload:   payload,
	}

	return m, nil
}

// String implements fmt.Stringer. It returns a string representation of the
// block.
func (b SkipBlock) String() string {
	return fmt.Sprintf("Block[%d:%v]", b.Index, b.hash)
}

// Fingerprint implements serde.Fingerprinter.
func (b SkipBlock) Fingerprint(w io.Writer) error {
	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, b.Index)
	_, err := w.Write(buffer)
	if err != nil {
		return xerrors.Errorf("couldn't write index: %v", err)
	}

	_, err = w.Write(b.GenesisID.Bytes())
	if err != nil {
		return xerrors.Errorf("couldn't write genesis hash: %v", err)
	}
	_, err = w.Write(b.BackLink.Bytes())
	if err != nil {
		return xerrors.Errorf("couldn't write backlink: %v", err)
	}

	err = b.Payload.Fingerprint(w)
	if err != nil {
		return xerrors.Errorf("couldn't fingerprint payload: %v", err)
	}

	return nil
}

// VerifiableBlock is a block combined with a consensus chain that can be
// verified from the genesis.
//
// - implements blockchain.VerifiableBlock
type VerifiableBlock struct {
	SkipBlock
	Chain consensus.Chain
}

// VisitJSON implements serde.Message. It serializes the verifiable block in
// JSON format.
func (vb VerifiableBlock) VisitJSON(ser serde.Serializer) (interface{}, error) {
	block, err := ser.Serialize(vb.SkipBlock)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize block: %v", err)
	}

	chain, err := ser.Serialize(vb.Chain)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize chain: %v", err)
	}

	m := json.VerifiableBlock{
		Block: block,
		Chain: chain,
	}

	return m, nil
}

// BlockFactory is responsible for the instantiation of the block and related
// data structures like the forward links and the proves.
//
// - implements blockchain.BlockFactory
type BlockFactory struct {
	serde.UnimplementedFactory

	hashFactory    crypto.HashFactory
	payloadFactory serde.Factory
}

// NewBlockFactory returns a new block factory that will use the factory for the
// payload.
func NewBlockFactory(f serde.Factory) BlockFactory {
	return BlockFactory{
		hashFactory:    crypto.NewSha256Factory(),
		payloadFactory: f,
	}
}

// VisitJSON implements serde.Message. It deserializes a block in JSON format.
func (f BlockFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.SkipBlock{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	var payload blockchain.Payload
	err = in.GetSerializer().Deserialize(m.Payload, f.payloadFactory, &payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize payload: %v", err)
	}

	block := SkipBlock{
		Index:   m.Index,
		Payload: payload,
	}

	copy(block.GenesisID[:], m.GenesisID)
	copy(block.BackLink[:], m.Backlink)

	h := f.hashFactory.New()
	err = block.Fingerprint(h)
	if err != nil {
		return nil, xerrors.Errorf("couldn't fingerprint block: %v", err)
	}

	copy(block.hash[:], h.Sum(nil))

	return block, nil
}

// VerifiableFactory is a message factory to deserialize verifiable block
// messages.
//
// - implements serde.Factory
type VerifiableFactory struct {
	serde.UnimplementedFactory

	blockFactory serde.Factory
	chainFactory serde.Factory
}

// NewVerifiableFactory returns a new verifiable block factory.
func NewVerifiableFactory(b, c serde.Factory) VerifiableFactory {
	return VerifiableFactory{
		blockFactory: b,
		chainFactory: c,
	}
}

// VisitJSON implements serde.Factory. It deserializes the verifiable block
// message in JSON format.
func (f VerifiableFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.VerifiableBlock{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	var chain consensus.Chain
	err = in.GetSerializer().Deserialize(m.Chain, f.chainFactory, &chain)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize chain: %v", err)
	}

	var block SkipBlock
	err = in.GetSerializer().Deserialize(m.Block, f.blockFactory, &block)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize block: %v", err)
	}

	vb := VerifiableBlock{
		SkipBlock: block,
		Chain:     chain,
	}

	return vb, nil
}
