package skipchain

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	fmt "fmt"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/crypto"
	"golang.org/x/xerrors"
)

const (
	// DefaultMaximumHeight is the default value when creating a genesis block
	// for the maximum height.
	DefaultMaximumHeight = 32

	// DefaultBaseHeight is the default value when creating a genesis block
	// for the base height.
	DefaultBaseHeight = 4
)

// BlockID is the type that defines the block identifier.
type BlockID []byte

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
		return nil, xerrors.Errorf("couldn't encode the header: %v", err)
	}

	payload, err := ptypes.MarshalAny(b.Payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode the payload: %v", err)
	}

	blockproto := &blockchain.Block{
		Index:   b.Index,
		Conodes: b.Roster.GetConodes(),
		Header:  header,
		Payload: payload,
	}

	return blockproto, nil
}

func (b SkipBlock) packHeader() (*any.Any, error) {
	fls := make([]*ForwardLinkProto, len(b.ForwardLinks))
	for i, fl := range b.ForwardLinks {
		flproto, err := fl.Pack()
		if err != nil {
			return nil, xerrors.Errorf("couldn't encode the forward link: %v", err)
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

	headerAny, err := ptypes.MarshalAny(header)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal to any: %v", err)
	}

	return headerAny, nil
}

func (b SkipBlock) computeHash() ([]byte, error) {
	h := sha256.New()

	buffer := make([]byte, 20)
	binary.LittleEndian.PutUint64(buffer[0:8], b.Index)
	binary.LittleEndian.PutUint32(buffer[8:12], b.Height)
	binary.LittleEndian.PutUint32(buffer[12:16], b.BaseHeight)
	binary.LittleEndian.PutUint32(buffer[16:20], b.MaximumHeight)
	h.Write(buffer)
	b.Roster.WriteTo(h)
	h.Write(b.GenesisID)
	h.Write(b.DataHash)

	for _, bl := range b.BackLinks {
		h.Write(bl)
	}

	return h.Sum(nil), nil
}

// ForwardLink is the cryptographic primitive to ensure a block is a successor
// of a previous one.
type ForwardLink struct {
	hash []byte
	From []byte
	To   []byte
	// Prepare signs the combination of From and To to prove that the nodes
	// agreed on a valid forward link between the two blocks.
	Prepare crypto.Signature
	// Commit signs the Prepare signature to prove that a threshold of the
	// nodes have committed the block.
	Commit crypto.Signature
}

// Verify makes sure the signatures of the forward link are correct.
func (fl ForwardLink) Verify(v crypto.Verifier, pubkeys []crypto.PublicKey) error {
	err := v.Verify(pubkeys, fl.hash, fl.Prepare)
	if err != nil {
		return xerrors.Errorf("couldn't verify the prepare signature: %v", err)
	}

	buffer, err := fl.Prepare.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("couldn't marshal the signature: %v", err)
	}

	err = v.Verify(pubkeys, buffer, fl.Commit)
	if err != nil {
		return xerrors.Errorf("coudln't verify the commit signature: %v", err)
	}

	return nil
}

// Pack returns the protobuf message of the forward link.
func (fl ForwardLink) Pack() (proto.Message, error) {
	flproto := &ForwardLinkProto{
		From: fl.From,
		To:   fl.To,
	}

	if fl.Prepare != nil {
		prepare, err := fl.Prepare.Pack()
		if err != nil {
			return nil, xerrors.Errorf("couldn't encode prepare: %v", err)
		}

		prepareAny, err := ptypes.MarshalAny(prepare)
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal prepare to any: %v", err)
		}

		flproto.Prepare = prepareAny
	}

	if fl.Commit != nil {
		commit, err := fl.Commit.Pack()
		if err != nil {
			return nil, xerrors.Errorf("couldn't encode commit: %v", err)
		}

		commitAny, err := ptypes.MarshalAny(commit)
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal commit to any: %v", err)
		}

		flproto.Commit = commitAny
	}

	return flproto, nil
}

func (fl ForwardLink) computeHash() ([]byte, error) {
	h := sha256.New()

	h.Write(fl.From)
	h.Write(fl.To)

	return h.Sum(nil), nil
}

// Chain is an ordered list of skipblocks.
type Chain []SkipBlock

// GetProof returns a verifiable proof to the latest block of the chain.
func (c Chain) GetProof() Proof {
	chain := c[:len(c)-1]

	proof := Proof{
		ForwardLinks: make([]ForwardLink, len(chain)),
	}

	for i, block := range chain {
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
			return nil, xerrors.Errorf("couldn't pack the forward link: %v", err)
		}

		proof.ForwardLinks[i] = packed.(*ForwardLinkProto)
	}

	return proof, nil
}

// Verify follows the chain from the beginning to insure the integrity.
func (p Proof) Verify(v crypto.Verifier, block SkipBlock) error {
	pubkeys := p.GenesisBlock.Roster.GetPublicKeys()

	if !bytes.Equal(p.ForwardLinks[0].From, p.GenesisBlock.hash) {
		fabric.Logger.Error().Msgf("%x != %x", p.ForwardLinks[0].From, p.GenesisBlock.hash)
		return xerrors.New("mismatching genesis hash in forward link")
	}

	for i, link := range p.ForwardLinks {
		if i > 0 {
			prev := p.ForwardLinks[i-1].To
			if !bytes.Equal(prev, link.From) {
				return xerrors.New("mismatching previous target in forward link")
			}
		}

		err := link.Verify(v, pubkeys)
		if err != nil {
			return xerrors.Errorf("couldn't verify the forward link: %v", err)
		}
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

	buffer, err := proto.Marshal(data)
	if err != nil {
		return SkipBlock{}, xerrors.Errorf("couldn't encode the data: %v", err)
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
	databuf, err := proto.Marshal(data)
	if err != nil {
		return SkipBlock{}, xerrors.Errorf("couldn't encode the data: %v", err)
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

func (f *blockFactory) fromForwardLink(src *ForwardLinkProto) (fl ForwardLink, err error) {
	fl.From = src.GetFrom()
	fl.To = src.GetTo()

	var sig crypto.Signature
	if src.GetPrepare() != nil {
		sig, err = f.verifier.GetSignatureFactory().FromAny(src.GetPrepare())
		if err != nil {
			err = xerrors.Errorf("couldn't decode prepare signature: %v", err)
			return
		}

		fl.Prepare = sig
	}

	if src.GetCommit() != nil {
		sig, err = f.verifier.GetSignatureFactory().FromAny(src.GetCommit())
		if err != nil {
			err = xerrors.Errorf("couldn't decode commit signature: %v", err)
			return
		}

		fl.Commit = sig
	}

	fl.hash, err = fl.computeHash()
	if err != nil {
		err = xerrors.Errorf("couldn't hash the forward link: %v", err)
	}

	return
}

func (f *blockFactory) fromBlock(src *blockchain.Block) (interface{}, error) {
	var header BlockHeaderProto
	err := ptypes.UnmarshalAny(src.GetHeader(), &header)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode header: %v", err)
	}

	var payload ptypes.DynamicAny
	err = ptypes.UnmarshalAny(src.GetPayload(), &payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode the payload: %v", err)
	}

	forwardLinks := make([]ForwardLink, len(header.GetForwardlinks()))
	for i, packed := range header.GetForwardlinks() {
		fl, err := f.fromForwardLink(packed)
		if err != nil {
			return nil, xerrors.Errorf("coudln't decode the forward link: %v", err)
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
	err := ptypes.UnmarshalAny(src, packed)
	if err != nil {
		return Proof{}, xerrors.Errorf("couldn't unpack proof: %v", err)
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

func (t *blockTriage) Verify(fl ForwardLink) error {
	block, ok := t.getBlock(fl.To)
	if !ok {
		return xerrors.Errorf("block not found: %x", fl.To)
	}

	pubkeys := block.Roster.GetPublicKeys()

	err := t.verifier.Verify(pubkeys, fl.hash, fl.Prepare)
	if err != nil {
		fabric.Logger.Err(err).Msg("found mismatching forward link")
	}

	return nil
}

func (t *blockTriage) Commit(fl ForwardLink) error {
	block, ok := t.getBlock(fl.To)
	if !ok {
		return xerrors.Errorf("block not found: %x", fl.To)
	}

	pubkeys := block.Roster.GetPublicKeys()

	msg, err := fl.Prepare.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("couldn't encode signature: %v", err)
	}

	err = t.verifier.Verify(pubkeys, msg, fl.Commit)
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
