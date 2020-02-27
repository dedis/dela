package skipchain

import (
	"crypto/sha256"
	"encoding/binary"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"go.dedis.ch/m/blockchain"
	"go.dedis.ch/m/crypto"
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
	GenesisID     []byte
	DataHash      []byte
	BackLinks     [][]byte
	ForwardLinks  []ForwardLink
	Payload       proto.Message
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

	header := &BlockHeaderProto{
		Height:        b.Height,
		BaseHeight:    b.BaseHeight,
		MaximumHeight: b.MaximumHeight,
		GenesisID:     b.GenesisID,
		DataHash:      b.DataHash,
		Backlinks:     b.BackLinks,
		Forwardlinks:  fls,
	}

	headerAny, err := ptypes.MarshalAny(header)
	if err != nil {
		return nil, err
	}

	return headerAny, nil
}

func (b SkipBlock) computeHash() error {
	h := sha256.New()

	buffer := make([]byte, 20)
	binary.LittleEndian.PutUint64(buffer[0:8], b.Index)
	binary.LittleEndian.PutUint32(buffer[8:12], b.Height)
	binary.LittleEndian.PutUint32(buffer[12:16], b.BaseHeight)
	binary.LittleEndian.PutUint32(buffer[16:20], b.MaximumHeight)
	h.Write(buffer)
	h.Write(b.GenesisID)
	h.Write(b.DataHash)

	for _, bl := range b.BackLinks {
		h.Write(bl)
	}

	b.hash = h.Sum(nil)

	return nil
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
		BackLinks:     [][]byte{},
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
		BackLinks:     [][]byte{prev.hash},
		ForwardLinks:  []ForwardLink{},
		Payload:       data,
	}

	err = block.computeHash()
	if err != nil {
		return nil, err
	}

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

	err = fl.computeHash()

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

	fls := make([]ForwardLink, len(header.GetForwardlinks()))
	for i, packed := range header.GetForwardlinks() {
		fl, err := f.fromForwardLink(packed)
		if err != nil {
			return nil, err
		}

		fls[i] = fl
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
		BackLinks:     header.GetBacklinks(),
		ForwardLinks:  fls,
		Payload:       payload.Message,
	}

	err = block.computeHash()
	if err != nil {
		return nil, err
	}

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

func (fl ForwardLink) computeHash() error {
	h := sha256.New()

	h.Write(fl.From)
	h.Write(fl.To)

	fl.hash = h.Sum(nil)

	return nil
}
