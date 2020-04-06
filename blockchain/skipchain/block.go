package skipchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	fmt "fmt"
	"hash"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

var (
	protoenc encoding.ProtoMarshaler = encoding.NewProtoEncoder()
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

// sha256Factory is a factory for SHA256 digests.
//
// - implements crypto.HashFactory
type sha256Factory struct{}

// New implements crypto.HashFactory. It returns a new instance of a SHA256
// hash.
func (f sha256Factory) New() hash.Hash {
	return sha256.New()
}

// SkipBlock is a representation of the data held by a block. It contains the
// information to build a skipchain.
//
// - implements blockchain.Block
// - implements consensus.Proposal
// - implements fmt.Stringer
type SkipBlock struct {
	hash     Digest
	verifier crypto.Verifier
	// Index is the block index since the genesis block.
	Index uint64
	// Conodes is the list of conodes participating in the consensus.
	Conodes Conodes
	// GenesisID is the hash of the genesis block which represents the chain
	// identifier.
	GenesisID Digest
	// BackLink is the hash of the previous block in the chain.
	BackLink Digest
	// Payload is the data stored in the block. It representation is independant
	// from the skipchain module.
	Payload proto.Message
}

func newSkipBlock(
	hashFactory crypto.HashFactory,
	verifier crypto.Verifier,
	index uint64,
	conodes Conodes,
	id Digest,
	backLink Digest,
	data proto.Message,
) (SkipBlock, error) {
	block := SkipBlock{
		verifier:  verifier,
		Index:     index,
		Conodes:   conodes,
		GenesisID: id,
		BackLink:  backLink,
		Payload:   data,
	}

	hash, err := block.computeHash(hashFactory)
	if err != nil {
		return SkipBlock{}, xerrors.Errorf("couldn't hash the block: %w", err)
	}

	block.hash = hash

	return block, nil
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

// GetPlayers implements blockchain.Block. It returns the list of players.
func (b SkipBlock) GetPlayers() mino.Players {
	return b.Conodes
}

// GetVerifier implements consensus.Proposal. It returns the verifier for the
// block.
// TODO: it might have sense to remove this function.
func (b SkipBlock) GetVerifier() crypto.Verifier {
	return b.verifier
}

// GetPayload implements blockchain.Block. It returns the block payload.
func (b SkipBlock) GetPayload() proto.Message {
	return b.Payload
}

// Pack implements encoding.Packable. It returns the protobuf message for a
// block.
func (b SkipBlock) Pack() (proto.Message, error) {
	payloadAny, err := protoenc.MarshalAny(b.Payload)
	if err != nil {
		return nil, encoding.NewAnyEncodingError(b.Payload, err)
	}

	roster, err := b.Conodes.Pack()
	if err != nil {
		return nil, encoding.NewEncodingError("conodes", err)
	}

	blockproto := &BlockProto{
		Index:     b.Index,
		GenesisID: b.GenesisID.Bytes(),
		Backlink:  b.BackLink.Bytes(),
		Payload:   payloadAny,
		Roster:    roster.(*Roster),
	}

	return blockproto, nil
}

// String implements fmt.Stringer. It returns a string representation of the
// block.
func (b SkipBlock) String() string {
	return fmt.Sprintf("Block[%v]", b.hash)
}

func (b SkipBlock) computeHash(factory crypto.HashFactory) (Digest, error) {
	h := factory.New()

	buffer := make([]byte, 20)
	binary.LittleEndian.PutUint64(buffer[0:8], b.Index)
	_, err := h.Write(buffer)
	if err != nil {
		return Digest{}, xerrors.Errorf("couldn't write index: %v", err)
	}

	if b.Conodes != nil {
		_, err = b.Conodes.WriteTo(h)
		if err != nil {
			return Digest{}, xerrors.Errorf("couldn't write conodes: %v", err)
		}
	}

	_, err = h.Write(b.GenesisID.Bytes())
	if err != nil {
		return Digest{}, xerrors.Errorf("couldn't write genesis hash: %v", err)
	}
	_, err = h.Write(b.BackLink.Bytes())
	if err != nil {
		return Digest{}, xerrors.Errorf("couldn't write backlink: %v", err)
	}

	if b.Payload != nil {
		databuf, err := protoenc.Marshal(b.Payload)
		if err != nil {
			return Digest{}, xerrors.Errorf("couldn't marshal payload: %w", err)
		}

		_, err = h.Write(databuf)
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

// Verify implements blockchain.VerifiableBlock. It makes sure the integrity of
// the chain is valid.
func (vb VerifiableBlock) Verify(v crypto.Verifier) error {
	err := vb.Chain.Verify(v)
	if err != nil {
		return xerrors.Errorf("couldn't verify the chain: %v", err)
	}

	if !bytes.Equal(vb.GetHash(), vb.Chain.GetLastHash()) {
		return xerrors.Errorf("mismatch target %x != %x", vb.GetHash(), vb.Chain.GetLastHash())
	}

	return nil
}

// Pack implements encoding.Packable. It returns the protobuf message for a
// verifiable block.
func (vb VerifiableBlock) Pack() (proto.Message, error) {
	block, err := vb.SkipBlock.Pack()
	if err != nil {
		return nil, encoding.NewEncodingError("block", err)
	}

	packed := &VerifiableBlockProto{
		Block: block.(*BlockProto),
	}

	packedChain, err := vb.Chain.Pack()
	if err != nil {
		return nil, encoding.NewEncodingError("chain", err)
	}
	packed.Chain, err = protoenc.MarshalAny(packedChain)
	if err != nil {
		return nil, encoding.NewAnyEncodingError(packedChain, err)
	}

	return packed, nil
}

// blockFactory is responsible for the instantiation of the block and related
// data structures like the forward links and the proves.
//
// - implements blockchain.BlockFactory
type blockFactory struct {
	*Skipchain
	hashFactory crypto.HashFactory
}

func (f blockFactory) fromPrevious(prev SkipBlock, data proto.Message) (SkipBlock, error) {
	var genesisID Digest
	if prev.Index == 0 {
		genesisID = prev.hash
	} else {
		genesisID = prev.GenesisID
	}

	block, err := newSkipBlock(
		f.hashFactory,
		prev.verifier,
		prev.Index+1,
		prev.Conodes,
		genesisID,
		prev.hash,
		data,
	)

	if err != nil {
		return block, xerrors.Errorf("couldn't make block: %w", err)
	}

	return block, nil
}

func (f blockFactory) decodeConodes(msgs []*ConodeProto) (Conodes, error) {
	pubkeyFactory := f.cosi.GetPublicKeyFactory()
	addrFactory := f.mino.GetAddressFactory()

	conodes := make(Conodes, len(msgs))
	for i, msg := range msgs {
		publicKey, err := pubkeyFactory.FromProto(msg.GetPublicKey())
		if err != nil {
			return nil, encoding.NewDecodingError("public key", err)
		}

		conodes[i] = Conode{
			addr:      addrFactory.FromText(msg.GetAddress()),
			publicKey: publicKey,
		}
	}
	return conodes, nil
}

func (f blockFactory) decodeBlock(src proto.Message) (SkipBlock, error) {
	in, ok := src.(*BlockProto)
	if !ok {
		return SkipBlock{}, xerrors.Errorf("invalid message type '%T'", src)
	}

	var payload ptypes.DynamicAny
	err := protoenc.UnmarshalAny(in.GetPayload(), &payload)
	if err != nil {
		return SkipBlock{}, encoding.NewAnyDecodingError(&payload, err)
	}

	backLink := Digest{}
	copy(backLink[:], in.GetBacklink())

	conodes, err := f.decodeConodes(in.GetRoster().GetConodes())
	if err != nil {
		return SkipBlock{}, xerrors.Errorf("couldn't create the conodes: %v", err)
	}

	verifier, err := f.cosi.GetVerifier(conodes)
	if err != nil {
		return SkipBlock{}, xerrors.Errorf("couldn't make verifier: %v", err)
	}

	genesisID := Digest{}
	copy(genesisID[:], in.GetGenesisID())

	block, err := newSkipBlock(
		f.hashFactory,
		verifier,
		in.GetIndex(),
		conodes,
		genesisID,
		backLink,
		payload.Message,
	)

	if err != nil {
		return block, xerrors.Errorf("couldn't make block: %v", err)
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

	chainFactory := f.consensus.GetChainFactory()

	chain, err := chainFactory.FromProto(in.GetChain())
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode the chain: %v", err)
	}

	err = VerifiableBlock{SkipBlock: block, Chain: chain}.Verify(block.verifier)
	if err != nil {
		return nil, xerrors.Errorf("couldn't verify: %v", err)
	}

	return block, nil
}
