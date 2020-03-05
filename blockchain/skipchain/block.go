package skipchain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	fmt "fmt"
	math "math"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/blockchain/consensus"
	"go.dedis.ch/fabric/blockchain/consensus/cosipbft"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"golang.org/x/xerrors"
)

const (
	// DefaultMaximumHeight is the default value when creating a genesis block
	// for the maximum height of each block.
	DefaultMaximumHeight = 32

	// DefaultBaseHeight is the default value when creating a genesis block
	// for the base height of each block.
	DefaultBaseHeight = 4
)

var (
	protoenc encoding.ProtoMarshaler = protoEncoder{}
)

// SkipBlock is a representation of the data held by a block. It contains the
// information to build a skipchain.
type SkipBlock struct {
	hash          []byte
	Index         uint64
	Roster        blockchain.Roster
	Height        uint32
	BaseHeight    uint32
	MaximumHeight uint32
	GenesisID     blockchain.BlockID
	DataHash      []byte
	BackLinks     []blockchain.BlockID
	Seals         []consensus.Seal
	Payload       proto.Message
}

// GetID returns the unique identifier for this block.
func (b SkipBlock) GetID() blockchain.BlockID {
	id := blockchain.BlockID{}
	copy(id[:], b.hash)
	return id
}

// Pack returns the protobuf message for a block.
func (b SkipBlock) Pack() (proto.Message, error) {
	payloadAny, err := protoenc.MarshalAny(b.Payload)
	if err != nil {
		return nil, encoding.NewAnyEncodingError(b.Payload, err)
	}

	seals := make([]*any.Any, len(b.Seals))
	for i, seal := range b.Seals {
		packed, err := seal.Pack()
		if err != nil {
			return nil, encoding.NewEncodingError("forward link", err)
		}

		seals[i], err = ptypes.MarshalAny(packed)
		if err != nil {
			return nil, err
		}
	}

	backLinks := make([][]byte, len(b.BackLinks))
	for i, bl := range b.BackLinks {
		backLinks[i] = bl.Bytes()
	}

	var conodes []*blockchain.Conode
	if b.Roster != nil {
		conodes, err = b.Roster.GetConodes()
		if err != nil {
			return nil, err
		}
	}

	blockproto := &BlockProto{
		Index:         b.Index,
		Height:        b.Height,
		BaseHeight:    b.BaseHeight,
		MaximumHeight: b.MaximumHeight,
		GenesisID:     b.GenesisID.Bytes(),
		DataHash:      b.DataHash,
		Backlinks:     backLinks,
		Seals:         seals,
		Payload:       payloadAny,
		Conodes:       conodes,
	}

	return blockproto, nil
}

func (b SkipBlock) String() string {
	return fmt.Sprintf("Block[%v]", b.GetID())
}

func (b SkipBlock) computeHash() ([]byte, error) {
	h := sha256.New()

	buffer := make([]byte, 20)
	binary.LittleEndian.PutUint64(buffer[0:8], b.Index)
	binary.LittleEndian.PutUint32(buffer[8:12], b.Height)
	binary.LittleEndian.PutUint32(buffer[12:16], b.BaseHeight)
	binary.LittleEndian.PutUint32(buffer[16:20], b.MaximumHeight)
	h.Write(buffer)
	if b.Roster != nil {
		b.Roster.WriteTo(h)
	}
	h.Write(b.GenesisID.Bytes())
	h.Write(b.DataHash)

	for _, bl := range b.BackLinks {
		h.Write(bl.Bytes())
	}

	return h.Sum(nil), nil
}

// SkipBlocks is an ordered list of skipblocks.
type SkipBlocks []SkipBlock

// GetProof returns a verifiable proof of the latest block of the chain.
func (c SkipBlocks) GetProof() Chain {
	numSeals := int(math.Max(0, float64(len(c)-1)))
	if len(c) == 0 {
		return Chain{}
	}

	proof := Chain{
		genesis: c[0],
		last:    c[len(c)-1],
		seals:   make([]consensus.Seal, numSeals),
	}

	for i, block := range c[:numSeals] {
		proof.seals[i] = block.Seals[0]
	}

	return proof
}

// Chain is a chain of forward links to a specific block so that the chain
// can be followed and verified.
type Chain struct {
	genesis SkipBlock
	last    SkipBlock
	seals   []consensus.Seal
}

// Pack implements the Packable interface to encode the proof into a protobuf
// message.
func (p Chain) Pack() (proto.Message, error) {
	block, err := p.last.Pack()
	if err != nil {
		return nil, err
	}

	proof := &ChainProto{
		Seals: make([]*any.Any, len(p.seals)),
		Block: block.(*BlockProto),
	}

	for i, seal := range p.seals {
		packed, err := seal.Pack()
		if err != nil {
			return nil, encoding.NewEncodingError("forward link", err)
		}

		proof.Seals[i], err = ptypes.MarshalAny(packed)
		if err != nil {
			return nil, err
		}
	}

	return proof, nil
}

// GetBlock returns the latest block of the chain.
func (p Chain) GetBlock() blockchain.Block {
	return p.last
}

// Verify follows the chain from the beginning to insure the integrity.
func (p Chain) Verify(v crypto.Verifier) error {
	if len(p.seals) == 0 {
		return nil
	}

	if p.genesis.Roster == nil {
		return xerrors.New("missing roster in genesis block")
	}

	pubkeys := p.genesis.Roster.GetPublicKeys()

	// Verify the first forward link against the genesis block.
	err := p.verifyLink(v, pubkeys, p.seals[0], p.genesis.GetID())
	if err != nil {
		return xerrors.Errorf("couldn't verify genesis forward link: %w", err)
	}

	// Then follow the chain of forward links.
	prev := p.seals[0].GetTo()
	for _, link := range p.seals[1:] {
		err := p.verifyLink(v, pubkeys, link, prev)
		if err != nil {
			return xerrors.Errorf("couldn't verify the forward link: %w", err)
		}

		prev = link.GetTo()
	}

	if !prev.Equal(p.last.GetID()) {
		return xerrors.Errorf("got forward link to %v but expect %v", prev, p.last.GetID())
	}

	return nil
}

func (p Chain) verifyLink(v crypto.Verifier, pubkeys []crypto.PublicKey, link consensus.Seal, prev blockchain.BlockID) error {
	err := link.Verify(v, pubkeys)
	if err != nil {
		return xerrors.Errorf("couldn't verify the signatures: %w", err)
	}

	if !prev.Equal(link.GetFrom()) {
		return xerrors.Errorf("got previous block %v but expect %v in forward link",
			link.GetFrom(), prev)
	}

	return nil
}

// blockFactory is responsible for the instantiation of the block and related
// data structures like the forward links and the proves.
type blockFactory struct {
	genesis     *SkipBlock
	verifier    crypto.Verifier
	sealFactory consensus.SealFactory
}

func newBlockFactory(v crypto.Verifier) *blockFactory {
	return &blockFactory{
		verifier:    v,
		sealFactory: cosipbft.NewSealFactory(v),
	}
}

func (f *blockFactory) createGenesis(roster blockchain.Roster, data proto.Message) (SkipBlock, error) {
	h := sha256.New()
	if data == nil {
		data = &empty.Empty{}
	}

	buffer, err := protoenc.Marshal(data)
	if err != nil {
		return SkipBlock{}, encoding.NewEncodingError("data", err)
	}

	h.Write(buffer)

	// TODO: crypto module for randomness
	randomBackLink := blockchain.BlockID{}
	rand.Read(randomBackLink[:])

	genesis := SkipBlock{
		Index:         0,
		Roster:        roster,
		Height:        1,
		BaseHeight:    DefaultBaseHeight,
		MaximumHeight: DefaultMaximumHeight,
		GenesisID:     blockchain.BlockID{},
		DataHash:      h.Sum(nil),
		BackLinks:     []blockchain.BlockID{randomBackLink},
		Seals:         []consensus.Seal{},
		Payload:       data,
	}

	genesis.hash, err = genesis.computeHash()
	if err != nil {
		return genesis, xerrors.Errorf("couldn't hash the genesis block: %v", err)
	}

	f.genesis = &genesis

	return genesis, nil
}

func (f *blockFactory) fromPrevious(prev SkipBlock, data proto.Message) (SkipBlock, error) {
	databuf, err := protoenc.Marshal(data)
	if err != nil {
		return SkipBlock{}, encoding.NewEncodingError("data", err)
	}

	h := sha256.New()
	h.Write(databuf)

	backlink := blockchain.BlockID{}
	copy(backlink[:], prev.hash)

	block := SkipBlock{
		Index:         prev.Index + 1,
		Roster:        prev.Roster,
		Height:        prev.Height,
		BaseHeight:    prev.BaseHeight,
		MaximumHeight: prev.MaximumHeight,
		GenesisID:     prev.GenesisID,
		DataHash:      h.Sum(nil),
		BackLinks:     []blockchain.BlockID{backlink},
		Seals:         []consensus.Seal{},
		Payload:       data,
	}

	hash, err := block.computeHash()
	if err != nil {
		return SkipBlock{}, xerrors.Errorf("couldn't hash the block: %v", err)
	}

	block.hash = hash

	return block, nil
}

func (f *blockFactory) fromBlock(src proto.Message) (SkipBlock, error) {
	in := src.(*BlockProto)

	var payload ptypes.DynamicAny
	err := protoenc.UnmarshalAny(in.GetPayload(), &payload)
	if err != nil {
		return SkipBlock{}, encoding.NewAnyDecodingError(&payload, err)
	}

	seals := make([]consensus.Seal, len(in.GetSeals()))
	for i, packed := range in.GetSeals() {
		seal, err := f.sealFactory.FromProto(packed)
		if err != nil {
			return SkipBlock{}, encoding.NewDecodingError("forward link", err)
		}

		seals[i] = seal
	}

	backLinks := make([]blockchain.BlockID, len(in.GetBacklinks()))
	for i, packed := range in.GetBacklinks() {
		backLinks[i] = blockchain.NewBlockID(packed)
	}

	roster, err := blockchain.NewRoster(f.verifier, in.GetConodes()...)
	if err != nil {
		return SkipBlock{}, xerrors.Errorf("couldn't create the roster: %v", err)
	}

	block := SkipBlock{
		Index:         in.GetIndex(),
		Roster:        roster,
		Height:        in.GetHeight(),
		BaseHeight:    in.GetBaseHeight(),
		MaximumHeight: in.GetMaximumHeight(),
		GenesisID:     blockchain.NewBlockID(in.GetGenesisID()),
		DataHash:      in.GetDataHash(),
		BackLinks:     backLinks,
		Seals:         seals,
		Payload:       payload.Message,
	}

	hash, err := block.computeHash()
	if err != nil {
		return SkipBlock{}, xerrors.Errorf("couldn't hash the block: %v", err)
	}

	block.hash = hash

	return block, nil
}

func (f *blockFactory) FromVerifiable(src proto.Message, roster blockchain.Roster) (blockchain.Block, error) {
	in, ok := src.(*ChainProto)
	if !ok {
		return nil, xerrors.New("unknown type")
	}

	if f.genesis == nil {
		return nil, xerrors.New("genesis block not initialized")
	}

	block, err := f.fromBlock(in.GetBlock())
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode the block: %v", err)
	}

	chain := Chain{
		genesis: *f.genesis,
		last:    block,
		seals:   make([]consensus.Seal, len(in.GetSeals())),
	}

	for i, sealproto := range in.GetSeals() {
		seal, err := f.sealFactory.FromProto(sealproto)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unpack forward link: %v", err)
		}

		chain.seals[i] = seal
	}

	err = chain.Verify(f.verifier)
	if err != nil {
		return nil, err
	}

	return block, nil
}

// PayloadValidator is the interface to implement to validate the payload
// when a new block is proposed.
type PayloadValidator interface {
	Validate(payload proto.Message) error
}

// The triage will determine which block must go through to be appended to
// the chain. It also acts as a buffer during the PBFT execution.
type blockTriage struct {
	factory   *blockFactory
	validator PayloadValidator
	db        Database
	blocks    []SkipBlock
}

func newBlockTriage(db Database, f *blockFactory, v PayloadValidator) *blockTriage {
	return &blockTriage{
		factory:   f,
		db:        db,
		validator: v,
	}
}

func (t *blockTriage) Prepare(in proto.Message) (blockchain.Block, error) {
	block, err := t.factory.fromBlock(in)

	last, err := t.db.ReadLast()
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the latest block: %v", err)
	}

	if block.Index <= last.Index {
		return nil, xerrors.Errorf("wrong index: %d <= %d", block.Index, last.Index)
	}

	err = t.validator.Validate(block.Payload)
	if err != nil {
		return nil, err
	}

	fabric.Logger.Debug().Msgf("Queuing block %v", block.GetID())
	t.blocks = append(t.blocks, block)

	return block, nil
}

func (t *blockTriage) getBlock(id blockchain.BlockID) (SkipBlock, bool) {
	for _, block := range t.blocks {
		if id.Equal(block.GetID()) {
			return block, true
		}
	}

	return SkipBlock{}, false
}

func (t *blockTriage) Commit(proposed blockchain.BlockID) error {
	block, ok := t.getBlock(proposed)
	if !ok {
		return xerrors.Errorf("block not found: %x", proposed)
	}

	fabric.Logger.Info().Msgf("Committed to block %v", block)

	return nil
}

func (t *blockTriage) processSeal(in proto.Message) error {
	seal, err := t.factory.sealFactory.FromProto(in)
	if err != nil {
		return err
	}

	block, ok := t.getBlock(seal.GetTo())
	if !ok {
		return xerrors.New("block not found")
	}

	// TODO: next OPs should be atomic.
	last, err := t.db.ReadLast()
	if err != nil {
		return xerrors.Errorf("couldn't read the latest block: %v", err)
	}

	err = t.db.Write(block)
	if err != nil {
		return xerrors.Errorf("couldn't update the block: %v", err)
	}

	last.Seals = []consensus.Seal{seal}
	err = t.db.Write(last)
	if err != nil {
		return xerrors.Errorf("couldn't update forward links: %v", err)
	}

	fabric.Logger.Debug().Msgf("Commit block %v", block.GetID())

	t.blocks = []SkipBlock{}

	return nil
}

type protoEncoder struct{}

func newProtoEncoder() protoEncoder {
	return protoEncoder{}
}

func (e protoEncoder) Marshal(pb proto.Message) ([]byte, error) {
	return proto.Marshal(pb)
}

func (e protoEncoder) MarshalAny(pb proto.Message) (*any.Any, error) {
	return ptypes.MarshalAny(pb)
}

func (e protoEncoder) UnmarshalAny(any *any.Any, pb proto.Message) error {
	return ptypes.UnmarshalAny(any, pb)
}
