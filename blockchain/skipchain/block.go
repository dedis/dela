package skipchain

import (
	"bytes"
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

// BlockID is the type that defines the block identifier.
type BlockID []byte

// String returns the hexadecimal value of the ID.
func (id BlockID) String() string {
	return fmt.Sprintf("%x", []byte(id))
}

// SkipBlock is a representation of the data held by a block. It contains the
// information to build a skipchain.
type SkipBlock struct {
	hash          []byte
	Index         uint64
	Roster        blockchain.Roster
	Height        uint32
	BaseHeight    uint32
	MaximumHeight uint32
	GenesisID     BlockID
	DataHash      []byte
	BackLinks     []BlockID
	ForwardLinks  []ForwardLink
	Payload       proto.Message
}

// GetID returns the unique identifier for this block.
func (b SkipBlock) GetID() BlockID {
	return b.hash
}

// Pack returns the protobuf message for a block.
func (b SkipBlock) Pack() (proto.Message, error) {
	header, err := b.packHeader()
	if err != nil {
		return nil, encoding.NewEncodingError("header", err)
	}

	headerAny, err := protoenc.MarshalAny(header)
	if err != nil {
		return nil, encoding.NewAnyEncodingError(header, err)
	}

	payloadAny, err := protoenc.MarshalAny(b.Payload)
	if err != nil {
		return nil, encoding.NewAnyEncodingError(b.Payload, err)
	}

	blockproto := &blockchain.Block{
		Index:   b.Index,
		Conodes: b.Roster.GetConodes(),
		Header:  headerAny,
		Payload: payloadAny,
	}

	return blockproto, nil
}

func (b SkipBlock) packHeader() (*BlockHeaderProto, error) {
	fls := make([]*ForwardLinkProto, len(b.ForwardLinks))
	for i, fl := range b.ForwardLinks {
		flproto, err := fl.Pack()
		if err != nil {
			return nil, encoding.NewEncodingError("forward link", err)
		}

		fls[i] = flproto.(*ForwardLinkProto)
	}

	backLinks := make([][]byte, len(b.BackLinks))
	for i, bl := range b.BackLinks {
		backLinks[i] = []byte(bl)
	}

	header := &BlockHeaderProto{
		Height:        b.Height,
		BaseHeight:    b.BaseHeight,
		MaximumHeight: b.MaximumHeight,
		GenesisID:     b.GenesisID,
		DataHash:      b.DataHash,
		Backlinks:     backLinks,
		Forwardlinks:  fls,
	}

	return header, nil
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
	h.Write(b.GenesisID)
	h.Write(b.DataHash)

	for _, bl := range b.BackLinks {
		h.Write(bl)
	}

	return h.Sum(nil), nil
}

// ForwardLink is an interface to represent a link between two blocks.
// TODO: make a consensus interface that is creating seals == links.
type ForwardLink interface {
	encoding.Packable

	GetFrom() BlockID
	GetTo() BlockID
	Verify(v crypto.Verifier, pubkeys []crypto.PublicKey) error
}

// forwardLink is the cryptographic primitive to ensure a block is a successor
// of a previous one.
type forwardLink struct {
	hash []byte
	from []byte
	to   []byte
	// prepare signs the combination of From and To to prove that the nodes
	// agreed on a valid forward link between the two blocks.
	prepare crypto.Signature
	// commit signs the Prepare signature to prove that a threshold of the
	// nodes have committed the block.
	commit crypto.Signature
}

func (fl forwardLink) GetFrom() BlockID {
	return BlockID(fl.from)
}

func (fl forwardLink) GetTo() BlockID {
	return BlockID(fl.to)
}

// Verify makes sure the signatures of the forward link are correct.
func (fl forwardLink) Verify(v crypto.Verifier, pubkeys []crypto.PublicKey) error {
	err := v.Verify(pubkeys, fl.hash, fl.prepare)
	if err != nil {
		return xerrors.Errorf("couldn't verify the prepare signature: %w", err)
	}

	buffer, err := fl.prepare.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("couldn't marshal the signature: %w", err)
	}

	err = v.Verify(pubkeys, buffer, fl.commit)
	if err != nil {
		return xerrors.Errorf("coudln't verify the commit signature: %w", err)
	}

	return nil
}

// Pack returns the protobuf message of the forward link.
func (fl forwardLink) Pack() (proto.Message, error) {
	flproto := &ForwardLinkProto{
		From: fl.from,
		To:   fl.to,
	}

	if fl.prepare != nil {
		prepare, err := fl.prepare.Pack()
		if err != nil {
			return nil, encoding.NewEncodingError("prepare", err)
		}

		prepareAny, err := protoenc.MarshalAny(prepare)
		if err != nil {
			return nil, encoding.NewAnyEncodingError(prepare, err)
		}

		flproto.Prepare = prepareAny
	}

	if fl.commit != nil {
		commit, err := fl.commit.Pack()
		if err != nil {
			return nil, encoding.NewEncodingError("commit", err)
		}

		commitAny, err := protoenc.MarshalAny(commit)
		if err != nil {
			return nil, encoding.NewAnyEncodingError(commit, err)
		}

		flproto.Commit = commitAny
	}

	return flproto, nil
}

func (fl forwardLink) computeHash() ([]byte, error) {
	h := sha256.New()

	h.Write(fl.from)
	h.Write(fl.to)

	return h.Sum(nil), nil
}

// Chain is an ordered list of skipblocks.
type Chain []SkipBlock

// GetProof returns a verifiable proof of the latest block of the chain.
func (c Chain) GetProof() Proof {
	numForwardLink := int(math.Max(0, float64(len(c)-1)))
	if len(c) == 0 {
		return Proof{}
	}

	proof := Proof{
		GenesisBlock: c[0],
		ForwardLinks: make([]ForwardLink, numForwardLink),
	}

	for i, block := range c[:numForwardLink] {
		proof.ForwardLinks[i] = block.ForwardLinks[0]
	}

	return proof
}

// Proof is a chain of forward links to a specific block so that the chain
// can be followed and verified.
type Proof struct {
	GenesisBlock SkipBlock
	ForwardLinks []ForwardLink
}

// Pack implements the Packable interface to encode the proof into a protobuf
// message.
func (p Proof) Pack() (proto.Message, error) {
	proof := &ProofProto{
		ForwardLinks: make([]*ForwardLinkProto, len(p.ForwardLinks)),
	}

	for i, link := range p.ForwardLinks {
		packed, err := link.Pack()
		if err != nil {
			return nil, encoding.NewEncodingError("forward link", err)
		}

		var ok bool
		proof.ForwardLinks[i], ok = packed.(*ForwardLinkProto)
		if !ok {
			return nil, encoding.NewTypeError(packed, (*ForwardLinkProto)(nil))
		}
	}

	return proof, nil
}

// Verify follows the chain from the beginning to insure the integrity.
func (p Proof) Verify(v crypto.Verifier, block SkipBlock) error {
	if len(p.ForwardLinks) == 0 {
		// No forward links so the genesis block is the only one.
		if !bytes.Equal(p.GenesisBlock.GetID(), block.GetID()) {
			return xerrors.New("mismatch genesis block")
		}

		return nil
	}

	if p.GenesisBlock.Roster == nil {
		return xerrors.New("missing roster in genesis block")
	}

	pubkeys := p.GenesisBlock.Roster.GetPublicKeys()

	// Verify the first forward link against the genesis block.
	err := p.verifyLink(v, pubkeys, p.ForwardLinks[0], p.GenesisBlock.GetID())
	if err != nil {
		return xerrors.Errorf("couldn't verify genesis forward link: %w", err)
	}

	// Then follow the chain of forward links.
	prev := p.ForwardLinks[0].GetTo()
	for _, link := range p.ForwardLinks[1:] {
		err := p.verifyLink(v, pubkeys, link, prev)
		if err != nil {
			return xerrors.Errorf("couldn't verify the forward link: %w", err)
		}

		prev = link.GetTo()
	}

	if !bytes.Equal(prev, block.GetID()) {
		return xerrors.Errorf("got forward link to %v but expect %v", prev, block.GetID())
	}

	return nil
}

func (p Proof) verifyLink(v crypto.Verifier, pubkeys []crypto.PublicKey, link ForwardLink, prev BlockID) error {
	err := link.Verify(v, pubkeys)
	if err != nil {
		return xerrors.Errorf("couldn't verify the signatures: %w", err)
	}

	if !bytes.Equal(prev, link.GetFrom()) {
		return xerrors.Errorf("got previous block %v but expect %v in forward link",
			link.GetFrom(), prev)
	}

	return nil
}

// blockFactory is responsible for the instantiation of the block and related
// data structures like the forward links and the proves.
type blockFactory struct {
	genesis  *SkipBlock
	verifier crypto.Verifier
}

func newBlockFactory(v crypto.Verifier) *blockFactory {
	return &blockFactory{
		verifier: v,
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
	randomBackLink := make([]byte, 32)
	rand.Read(randomBackLink)

	genesis := SkipBlock{
		Index:         0,
		Roster:        roster,
		Height:        1,
		BaseHeight:    DefaultBaseHeight,
		MaximumHeight: DefaultMaximumHeight,
		GenesisID:     nil,
		DataHash:      h.Sum(nil),
		BackLinks:     []BlockID{randomBackLink},
		ForwardLinks:  []ForwardLink{},
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

	block := SkipBlock{
		Index:         prev.Index + 1,
		Roster:        prev.Roster,
		Height:        prev.Height,
		BaseHeight:    prev.BaseHeight,
		MaximumHeight: prev.MaximumHeight,
		GenesisID:     prev.GenesisID,
		DataHash:      h.Sum(nil),
		BackLinks:     []BlockID{prev.hash},
		ForwardLinks:  []ForwardLink{},
		Payload:       data,
	}

	hash, err := block.computeHash()
	if err != nil {
		return SkipBlock{}, xerrors.Errorf("couldn't hash the block: %v", err)
	}

	block.hash = hash

	return block, nil
}

func (f *blockFactory) fromForwardLink(src *ForwardLinkProto) (ForwardLink, error) {
	fl := forwardLink{
		from: src.GetFrom(),
		to:   src.GetTo(),
	}

	if src.GetPrepare() != nil {
		sig, err := f.verifier.GetSignatureFactory().FromProto(src.GetPrepare())
		if err != nil {
			return nil, encoding.NewDecodingError("prepare signature", err)
		}

		fl.prepare = sig
	}

	if src.GetCommit() != nil {
		sig, err := f.verifier.GetSignatureFactory().FromProto(src.GetCommit())
		if err != nil {
			return nil, encoding.NewDecodingError("commit signature", err)
		}

		fl.commit = sig
	}

	hash, err := fl.computeHash()
	if err != nil {
		return nil, xerrors.Errorf("couldn't hash the forward link: %v", err)
	}

	fl.hash = hash

	return fl, nil
}

func (f *blockFactory) fromBlock(src *blockchain.Block) (interface{}, error) {
	var header BlockHeaderProto
	err := protoenc.UnmarshalAny(src.GetHeader(), &header)
	if err != nil {
		return nil, encoding.NewAnyDecodingError(&header, err)
	}

	var payload ptypes.DynamicAny
	err = protoenc.UnmarshalAny(src.GetPayload(), &payload)
	if err != nil {
		return nil, encoding.NewAnyDecodingError(&payload, err)
	}

	forwardLinks := make([]ForwardLink, len(header.GetForwardlinks()))
	for i, packed := range header.GetForwardlinks() {
		fl, err := f.fromForwardLink(packed)
		if err != nil {
			return nil, encoding.NewDecodingError("forward link", err)
		}

		forwardLinks[i] = fl
	}

	backLinks := make([]BlockID, len(header.GetBacklinks()))
	for i, packed := range header.GetBacklinks() {
		backLinks[i] = BlockID(packed)
	}

	roster, err := blockchain.NewRoster(f.verifier, src.GetConodes()...)
	if err != nil {
		return nil, xerrors.Errorf("couldn't create the roster: %v", err)
	}

	block := SkipBlock{
		Index:         src.GetIndex(),
		Roster:        roster,
		Height:        header.GetHeight(),
		BaseHeight:    header.GetBaseHeight(),
		MaximumHeight: header.GetMaximumHeight(),
		GenesisID:     header.GetGenesisID(),
		DataHash:      header.GetDataHash(),
		BackLinks:     backLinks,
		ForwardLinks:  forwardLinks,
		Payload:       payload.Message,
	}

	hash, err := block.computeHash()
	if err != nil {
		return nil, xerrors.Errorf("couldn't hash the block: %v", err)
	}

	block.hash = hash

	return block, nil
}

func (f *blockFactory) fromProof(src *any.Any) (Proof, error) {
	packed := &ProofProto{}
	err := protoenc.UnmarshalAny(src, packed)
	if err != nil {
		return Proof{}, encoding.NewAnyDecodingError(packed, err)
	}

	proof := Proof{
		GenesisBlock: *f.genesis,
		ForwardLinks: make([]ForwardLink, len(packed.GetForwardLinks())),
	}

	for i, fl := range packed.GetForwardLinks() {
		forwardLink, err := f.fromForwardLink(fl)
		if err != nil {
			return proof, xerrors.Errorf("couldn't unpack forward link: %v", err)
		}

		proof.ForwardLinks[i] = forwardLink
	}

	return proof, nil
}

func (f *blockFactory) FromVerifiable(src *blockchain.VerifiableBlock, pubkeys []crypto.PublicKey) (interface{}, error) {
	block, err := f.fromBlock(src.GetBlock())
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode the block: %v", err)
	}

	if f.genesis == nil {
		return nil, xerrors.New("genesis block not initialized")
	}

	proof, err := f.fromProof(src.GetProof())
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode the proof: %v", err)
	}

	err = proof.Verify(f.verifier, block.(SkipBlock))
	if err != nil {
		return nil, xerrors.Errorf("couldn't verify the chain: %v", err)
	}

	return block, nil
}

// The triage will determine which block must go through to be appended to
// the chain. It also acts as a buffer during the PBFT execution.
type blockTriage struct {
	db       Database
	verifier crypto.Verifier
	blocks   []SkipBlock
}

func newBlockTriage(db Database, v crypto.Verifier) *blockTriage {
	return &blockTriage{
		db:       db,
		verifier: v,
	}
}

func (t *blockTriage) Add(block SkipBlock) error {
	last, err := t.db.ReadLast()
	if err != nil {
		return xerrors.Errorf("couldn't read the latest block: %v", err)
	}

	if block.Index <= last.Index {
		return xerrors.Errorf("wrong index: %d <= %d", block.Index, last.Index)
	}

	fabric.Logger.Debug().Msgf("Queuing block %v", block.GetID())
	t.blocks = append(t.blocks, block)

	return nil
}

func (t *blockTriage) getBlock(id BlockID) (SkipBlock, bool) {
	for _, block := range t.blocks {
		if bytes.Equal(block.hash, id) {
			return block, true
		}
	}

	return SkipBlock{}, false
}

func (t *blockTriage) Verify(fl forwardLink) error {
	block, ok := t.getBlock(fl.to)
	if !ok {
		return xerrors.Errorf("block not found: %x", fl.to)
	}

	pubkeys := block.Roster.GetPublicKeys()

	err := t.verifier.Verify(pubkeys, fl.hash, fl.prepare)
	if err != nil {
		fabric.Logger.Err(err).Msg("found mismatching forward link")
	}

	return nil
}

func (t *blockTriage) Commit(fl forwardLink) error {
	block, ok := t.getBlock(fl.to)
	if !ok {
		return xerrors.Errorf("block not found: %x", fl.to)
	}

	pubkeys := block.Roster.GetPublicKeys()

	msg, err := fl.prepare.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("couldn't encode signature: %v", err)
	}

	err = t.verifier.Verify(pubkeys, msg, fl.commit)
	if err != nil {
		return xerrors.Errorf("couldn't verify the forward link: %v", err)
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

	last.ForwardLinks = []ForwardLink{fl}
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
