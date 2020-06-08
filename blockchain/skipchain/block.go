package skipchain

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/mino"
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
	hash Digest

	// Origin is the address of the block creator.
	Origin mino.Address

	// Index is the block index since the genesis block.
	Index uint64

	// GenesisID is the hash of the genesis block which represents the chain
	// identifier.
	GenesisID Digest

	// BackLink is the hash of the previous block in the chain.
	BackLink Digest

	// Payload is the data stored in the block. It representation is independant
	// from the skipchain module.
	Payload proto.Message
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

// GetPreviousHash implements consensus.Proposal. It returns the previous block
// digest.
func (b SkipBlock) GetPreviousHash() []byte {
	return b.BackLink.Bytes()
}

// GetPayload implements blockchain.Block. It returns the block payload.
func (b SkipBlock) GetPayload() proto.Message {
	return b.Payload
}

// Pack implements encoding.Packable. It returns the protobuf message for a
// block.
func (b SkipBlock) Pack(encoder encoding.ProtoMarshaler) (proto.Message, error) {
	payloadAny, err := encoder.MarshalAny(b.Payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal the payload: %v", err)
	}

	origin, err := b.Origin.MarshalText()
	if err != nil {
		return nil, err
	}

	blockproto := &BlockProto{
		Origin:    origin,
		Index:     b.Index,
		GenesisID: b.GenesisID.Bytes(),
		Backlink:  b.BackLink.Bytes(),
		Payload:   payloadAny,
	}

	return blockproto, nil
}

// String implements fmt.Stringer. It returns a string representation of the
// block.
func (b SkipBlock) String() string {
	return fmt.Sprintf("Block[%d:%v]", b.Index, b.hash)
}

func (b SkipBlock) computeHash(factory crypto.HashFactory,
	enc encoding.ProtoMarshaler) (Digest, error) {

	h := factory.New()

	buffer := make([]byte, 20)
	binary.LittleEndian.PutUint64(buffer[0:8], b.Index)
	_, err := h.Write(buffer)
	if err != nil {
		return Digest{}, xerrors.Errorf("couldn't write index: %v", err)
	}

	if b.Origin == nil {
		return Digest{}, xerrors.New("missing block origin")
	}

	buffer, err = b.Origin.MarshalText()
	if err != nil {
		return Digest{}, xerrors.Errorf("couldn't marshal origin: %v", err)
	}

	_, err = h.Write(buffer)
	if err != nil {
		return Digest{}, xerrors.Errorf("couldn't write origin: %v", err)
	}

	_, err = h.Write(b.GenesisID.Bytes())
	if err != nil {
		return Digest{}, xerrors.Errorf("couldn't write genesis hash: %v", err)
	}
	_, err = h.Write(b.BackLink.Bytes())
	if err != nil {
		return Digest{}, xerrors.Errorf("couldn't write backlink: %v", err)
	}

	if proto.Size(b.Payload) > 0 {
		err := enc.MarshalStable(h, b.Payload)
		if err != nil {
			return Digest{}, xerrors.Errorf("couldn't write payload: %v", err)
		}
	}

	digest := Digest{}
	copy(digest[:], h.Sum(nil))

	return digest, nil
}

// VerifiableBlock is a block combined with a consensus chain that can be
// verified from the genesis.
//
// - implements blockchain.VerifiableBlock
type VerifiableBlock struct {
	SkipBlock
	Chain consensus.Chain
}

// Pack implements encoding.Packable. It returns the protobuf message for a
// verifiable block.
func (vb VerifiableBlock) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	block, err := enc.Pack(vb.SkipBlock)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack block: %v", err)
	}

	packed := &VerifiableBlockProto{
		Block: block.(*BlockProto),
	}

	packed.Chain, err = enc.PackAny(vb.Chain)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack chain: %v", err)
	}

	return packed, nil
}

// blockFactory is responsible for the instantiation of the block and related
// data structures like the forward links and the proves.
//
// - implements blockchain.BlockFactory
type blockFactory struct {
	encoder     encoding.ProtoMarshaler
	hashFactory crypto.HashFactory
	consensus   consensus.Consensus
	mino        mino.Mino
}

func (f blockFactory) prepareBlock(block *SkipBlock) error {
	hash, err := block.computeHash(f.hashFactory, f.encoder)
	if err != nil {
		return xerrors.Errorf("couldn't hash the block: %w", err)
	}

	block.hash = hash

	return nil
}

func (f blockFactory) fromPrevious(prev SkipBlock, data proto.Message) (SkipBlock, error) {
	var genesisID Digest
	if prev.Index == 0 {
		genesisID = prev.hash
	} else {
		genesisID = prev.GenesisID
	}

	block := SkipBlock{
		Origin:    f.mino.GetAddress(),
		Index:     prev.Index + 1,
		GenesisID: genesisID,
		BackLink:  prev.hash,
		Payload:   data,
	}

	err := f.prepareBlock(&block)
	if err != nil {
		return block, xerrors.Errorf("couldn't make block: %w", err)
	}

	return block, nil
}

func (f blockFactory) decodeBlock(src proto.Message) (SkipBlock, error) {
	in, ok := src.(*BlockProto)
	if !ok {
		return SkipBlock{}, xerrors.Errorf("invalid message type '%T'", src)
	}

	payload, err := f.encoder.UnmarshalDynamicAny(in.GetPayload())
	if err != nil {
		return SkipBlock{}, xerrors.Errorf("couldn't unmarshal payload: %v", err)
	}

	backLink := Digest{}
	copy(backLink[:], in.GetBacklink())

	genesisID := Digest{}
	copy(genesisID[:], in.GetGenesisID())

	block := SkipBlock{
		Origin:    f.mino.GetAddressFactory().FromText(in.Origin),
		Index:     in.GetIndex(),
		GenesisID: genesisID,
		BackLink:  backLink,
		Payload:   payload,
	}

	err = f.prepareBlock(&block)
	if err != nil {
		return block, xerrors.Errorf("couldn't prepare block: %v", err)
	}

	return block, nil
}

// FromVerifiable implements blockchain.BlockFactory. It returns the block if
// the integrity of the message is verified.
func (f blockFactory) FromVerifiable(src proto.Message) (blockchain.Block, error) {
	in, ok := src.(*VerifiableBlockProto)
	if !ok {
		return nil, xerrors.Errorf("invalid message type '%T'", src)
	}

	block, err := f.decodeBlock(in.GetBlock())
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode the block: %v", err)
	}

	chainFactory, err := f.consensus.GetChainFactory()
	if err != nil {
		return nil, xerrors.Errorf("couldn't get the chain factory: %v", err)
	}

	// Integrity of the chain is verified during decoding.
	chain, err := chainFactory.FromProto(in.GetChain())
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode the chain: %v", err)
	}

	// Only the link between the chain and the block needs to be verified.
	if !bytes.Equal(chain.GetTo(), block.hash[:]) {
		return nil, xerrors.Errorf("mismatch hashes: %#x != %#x",
			chain.GetTo(), block.GetHash())
	}

	return block, nil
}
