package types

import (
	"encoding/binary"
	"fmt"
	"io"

	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var (
	blockFormats      = registry.NewSimpleRegistry()
	verifiableFormats = registry.NewSimpleRegistry()
)

func RegisterBlockFormat(c serde.Format, f serde.FormatEngine) {
	blockFormats.Register(c, f)
}

func RegisterVerifiableBlockFormats(c serde.Format, f serde.FormatEngine) {
	verifiableFormats.Register(c, f)
}

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

type emptyPayload struct{}

func (p emptyPayload) Serialize(serde.Context) ([]byte, error) {
	return nil, nil
}

func (p emptyPayload) Fingerprint(io.Writer) error {
	return nil
}

// SkipBlock is a representation of the data held by a block. It contains the
// information to build a skipchain.
//
// - implements blockchain.Block
// - implements consensus.Proposal
// - implements fmt.Stringer
type SkipBlock struct {
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

type skipBlockTemplate struct {
	SkipBlock

	hashFactory crypto.HashFactory
}

type SkipBlockOption func(*skipBlockTemplate)

func WithIndex(index uint64) SkipBlockOption {
	return func(tmpl *skipBlockTemplate) {
		tmpl.Index = index
	}
}

func WithGenesisID(id []byte) SkipBlockOption {
	return func(tmpl *skipBlockTemplate) {
		copy(tmpl.GenesisID[:], id)
	}
}

func WithBackLink(id []byte) SkipBlockOption {
	return func(tmpl *skipBlockTemplate) {
		copy(tmpl.BackLink[:], id)
	}
}

func WithPayload(payload blockchain.Payload) SkipBlockOption {
	return func(tmpl *skipBlockTemplate) {
		tmpl.Payload = payload
	}
}

func WithHashFactory(f crypto.HashFactory) SkipBlockOption {
	return func(tmpl *skipBlockTemplate) {
		tmpl.hashFactory = f
	}
}

func NewSkipBlock(opts ...SkipBlockOption) (SkipBlock, error) {
	tmpl := skipBlockTemplate{
		SkipBlock: SkipBlock{
			Payload: emptyPayload{},
		},
		hashFactory: crypto.NewSha256Factory(),
	}

	for _, opt := range opts {
		opt(&tmpl)
	}

	h := tmpl.hashFactory.New()
	err := tmpl.Fingerprint(h)
	if err != nil {
		return tmpl.SkipBlock, err
	}

	copy(tmpl.hash[:], h.Sum(nil))

	return tmpl.SkipBlock, nil
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

// Serialize implements serde.Message.
func (b SkipBlock) Serialize(ctx serde.Context) ([]byte, error) {
	format := blockFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, b)
	if err != nil {
		return nil, err
	}

	return data, nil
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

// Serialize implements serde.Message.
func (vb VerifiableBlock) Serialize(ctx serde.Context) ([]byte, error) {
	format := verifiableFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, vb)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type PayloadKey struct{}

// BlockFactory is responsible for the instantiation of the block and related
// data structures like the forward links and the proves.
//
// - implements blockchain.BlockFactory
type BlockFactory struct {
	payloadFactory serde.Factory
}

// NewBlockFactory returns a new block factory that will use the factory for the
// payload.
func NewBlockFactory(f serde.Factory) BlockFactory {
	return BlockFactory{
		payloadFactory: f,
	}
}

// Deserialize implements serde.Message.
func (f BlockFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := blockFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, PayloadKey{}, f.payloadFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

type ChainKey struct{}

// VerifiableFactory is a message factory to deserialize verifiable block
// messages.
//
// - implements serde.Factory
type VerifiableFactory struct {
	payloadFactory serde.Factory
	chainFactory   consensus.ChainFactory
}

// NewVerifiableFactory returns a new verifiable block factory.
func NewVerifiableFactory(cf consensus.ChainFactory, pf serde.Factory) VerifiableFactory {
	return VerifiableFactory{
		payloadFactory: pf,
		chainFactory:   cf,
	}
}

// Deserialize implements serde.Factory.
func (f VerifiableFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := verifiableFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, PayloadKey{}, f.payloadFactory)
	ctx = serde.WithFactory(ctx, ChainKey{}, f.chainFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
