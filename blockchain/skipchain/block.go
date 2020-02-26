package skipchain

import (
	"crypto/sha256"
	"encoding/binary"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/m/blockchain"
	"go.dedis.ch/m/crypto"
)

// SkipBlock is a representation of the data held by a block. It contains the
// information to build a skipchain.
type SkipBlock struct {
	hash    []byte
	Index   uint64
	Roster  blockchain.Roster
	Header  *SkipBlockHeader
	Payload proto.Message
}

// Pack returns the protobuf message for a block.
func (b SkipBlock) Pack() (proto.Message, error) {
	header, err := ptypes.MarshalAny(b.Header)
	if err != nil {
		return nil, err
	}

	payload, err := ptypes.MarshalAny(b.Payload)
	if err != nil {
		return nil, err
	}

	blockproto := &blockchain.Block{
		Index:   b.Index,
		Roster:  b.Roster,
		Header:  header,
		Payload: payload,
	}

	return blockproto, nil
}

func (b SkipBlock) computeHash() error {
	h := sha256.New()

	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, b.Index)
	_, err := h.Write(buffer)
	if err != nil {
		return err
	}

	buffer, err = proto.Marshal(b.Header)
	if err != nil {
		return err
	}

	_, err = h.Write(buffer)
	if err != nil {
		return err
	}

	return nil
}

type blockFactory struct{}

func (f blockFactory) createGenesis(roster blockchain.Roster) SkipBlock {
	return SkipBlock{
		Index:  0,
		Roster: roster,
		Header: &SkipBlockHeader{
			Height:        1,
			BaseHeight:    4,
			MaximumHeight: 32,
			GenesisID:     nil,
			DataHash:      nil,
			Backlinks:     [][]byte{},
			Forwardlinks:  []*ForwardLink{},
		},
		Payload: nil,
	}
}

func (f blockFactory) FromPrevious(previous interface{}, data proto.Message) (interface{}, error) {
	prev := previous.(SkipBlock)

	databuf, err := proto.Marshal(data)
	if err != nil {
		return nil, err
	}

	h := sha256.New()
	_, err = h.Write(databuf)
	if err != nil {
		return nil, err
	}

	header := proto.Clone(prev.Header).(*SkipBlockHeader)
	header.Backlinks = [][]byte{prev.hash}
	header.DataHash = h.Sum(nil)

	block := SkipBlock{
		Index:   prev.Index + 1,
		Roster:  prev.Roster,
		Header:  header,
		Payload: data,
	}

	err = block.computeHash()
	if err != nil {
		return nil, err
	}

	return block, nil
}

func (f blockFactory) fromBlock(src *blockchain.Block) (interface{}, error) {
	var header SkipBlockHeader
	err := ptypes.UnmarshalAny(src.GetHeader(), &header)
	if err != nil {
		return nil, err
	}

	var payload ptypes.DynamicAny
	err = ptypes.UnmarshalAny(src.GetPayload(), &payload)
	if err != nil {
		return nil, err
	}

	block := SkipBlock{
		Index:   src.GetIndex(),
		Roster:  src.GetRoster(),
		Header:  &header,
		Payload: payload.Message,
	}

	err = block.computeHash()
	if err != nil {
		return nil, err
	}

	return block, nil
}

func (f blockFactory) FromVerifiable(src *blockchain.VerifiableBlock, pubkeys []crypto.PublicKey) (interface{}, error) {
	return SkipBlock{}, nil
}
