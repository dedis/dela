package skipchain

import (
	"bytes"
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
		return nil, err
	}

	payload, err := ptypes.MarshalAny(b.Payload)
	if err != nil {
		return nil, err
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
			return nil, err
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
		return nil, err
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
	hash    []byte
	From    []byte
	To      []byte
	Prepare crypto.Signature
	Commit  crypto.Signature
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
			return nil, err
		}

		prepareAny, err := ptypes.MarshalAny(prepare)
		if err != nil {
			return nil, err
		}

		flproto.Prepare = prepareAny
	}

	if fl.Commit != nil {
		commit, err := fl.Commit.Pack()
		if err != nil {
			return nil, err
		}

		commitAny, err := ptypes.MarshalAny(commit)
		if err != nil {
			return nil, err
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

type blockFactory struct {
	verifier crypto.Verifier
}

func newBlockFactory(v crypto.Verifier) blockFactory {
	return blockFactory{
		verifier: v,
	}
}

func (f blockFactory) createGenesis(roster blockchain.Roster) SkipBlock {
	return SkipBlock{
		Index:         0,
		Roster:        roster,
		Height:        1,
		BaseHeight:    4,
		MaximumHeight: 32,
		GenesisID:     []byte{},
		DataHash:      []byte{},
		BackLinks:     []BlockID{},
		ForwardLinks:  []ForwardLink{},
		Payload:       &empty.Empty{},
	}
}

func (f blockFactory) FromPrevious(previous interface{}, data proto.Message) (interface{}, error) {
	prev := previous.(SkipBlock)

	databuf, err := proto.Marshal(data)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	block.hash = hash

	return block, nil
}

func (f blockFactory) fromForwardLink(src *ForwardLinkProto) (fl ForwardLink, err error) {
	fl.From = src.GetFrom()
	fl.To = src.GetTo()

	var sig crypto.Signature
	if src.GetPrepare() != nil {
		sig, err = f.verifier.GetSignatureFactory().FromAny(src.GetPrepare())
		if err != nil {
			return
		}

		fl.Prepare = sig
	}

	if src.GetCommit() != nil {
		sig, err = f.verifier.GetSignatureFactory().FromAny(src.GetCommit())
		if err != nil {
			return
		}

		fl.Commit = sig
	}

	fl.hash, err = fl.computeHash()

	return
}

func (f blockFactory) fromBlock(src *blockchain.Block) (interface{}, error) {
	var header BlockHeaderProto
	err := ptypes.UnmarshalAny(src.GetHeader(), &header)
	if err != nil {
		return nil, err
	}

	var payload ptypes.DynamicAny
	err = ptypes.UnmarshalAny(src.GetPayload(), &payload)
	if err != nil {
		return nil, err
	}

	forwardLinks := make([]ForwardLink, len(header.GetForwardlinks()))
	for i, packed := range header.GetForwardlinks() {
		fl, err := f.fromForwardLink(packed)
		if err != nil {
			return nil, err
		}

		forwardLinks[i] = fl
	}

	backLinks := make([]BlockID, len(header.GetBacklinks()))
	for i, packed := range header.GetBacklinks() {
		backLinks[i] = BlockID(packed)
	}

	roster, err := blockchain.NewRoster(f.verifier, src.GetConodes()...)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	block.hash = hash

	return block, nil
}

func (f blockFactory) FromVerifiable(src *blockchain.VerifiableBlock, pubkeys []crypto.PublicKey) (interface{}, error) {
	// TODO: verify chain

	block, err := f.fromBlock(src.GetBlock())
	if err != nil {
		return nil, err
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
		return err
	}

	if block.Index <= last.Index {
		return fmt.Errorf("wrong index: %d <= %d", block.Index, last.Index)
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
		return fmt.Errorf("block not found: %x", fl.To)
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
		return fmt.Errorf("block not found: %x", fl.To)
	}

	pubkeys := block.Roster.GetPublicKeys()

	msg, err := fl.Prepare.MarshalBinary()
	if err != nil {
		return err
	}

	err = t.verifier.Verify(pubkeys, msg, fl.Commit)
	if err != nil {
		fabric.Logger.Err(err).Msg("found mismatching forward link")
	}

	last, err := t.db.ReadLast()
	if err != nil {
		return err
	}

	err = t.db.Write(block)
	if err != nil {
		return err
	}

	last.ForwardLinks = []ForwardLink{fl}
	err = t.db.Write(last)
	if err != nil {
		return err
	}

	fabric.Logger.Debug().Msgf("Commit block %v", block.GetID())

	t.blocks = []SkipBlock{}

	return nil
}
