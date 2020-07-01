package types

import (
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
)

var (
	bpFormats      = registry.NewSimpleRegistry()
	requestFormats = registry.NewSimpleRegistry()
)

func RegisterBlueprintFormats(c serde.Format, f serde.FormatEngine) {
	bpFormats.Register(c, f)
}

func RegisterRequestFormats(c serde.Format, f serde.FormatEngine) {
	requestFormats.Register(c, f)
}

// Blueprint is the proposition sent to create a new block.
//
// - implements serde.Message
type Blueprint struct {
	index    uint64
	previous Digest
	data     serde.Message
}

func NewBlueprint(index uint64, previous []byte, data serde.Message) Blueprint {
	bp := Blueprint{
		index: index,
		data:  data,
	}

	copy(bp.previous[:], previous)

	return bp
}

func (b Blueprint) GetIndex() uint64 {
	return b.index
}

func (b Blueprint) GetPrevious() []byte {
	return b.previous[:]
}

func (b Blueprint) GetData() serde.Message {
	return b.data
}

// Serialize implements serde.Message.
func (b Blueprint) Serialize(ctx serde.Context) ([]byte, error) {
	format := bpFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, b)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type DataKey struct{}

// BlueprintFactory is a factory for the blueprint message.
//
// - implements serde.Factory
type BlueprintFactory struct {
	factory serde.Factory
}

func NewBlueprintFactory(f serde.Factory) BlueprintFactory {
	return BlueprintFactory{
		factory: f,
	}
}

// Deserialize implements serde.Factory.
func (f BlueprintFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := bpFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, DataKey{}, f.factory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

// PropagateGenesis is the message sent to share the genesis block.
//
// - implements serde.Message
type PropagateGenesis struct {
	genesis SkipBlock
}

func NewPropagateGenesis(genesis SkipBlock) PropagateGenesis {
	return PropagateGenesis{
		genesis: genesis,
	}
}

func (p PropagateGenesis) GetGenesis() SkipBlock {
	return p.genesis
}

// Serialize implements serde.Message.
func (p PropagateGenesis) Serialize(ctx serde.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, p)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// BlockRequest is the message sent to request a block.
//
// - implements serde.Message
type BlockRequest struct {
	from uint64
	to   uint64
}

func NewBlockRequest(from, to uint64) BlockRequest {
	return BlockRequest{
		from: from,
		to:   to,
	}
}

func (req BlockRequest) GetFrom() uint64 {
	return req.from
}

func (req BlockRequest) GetTo() uint64 {
	return req.to
}

// Serialize implements serde.Message.
func (req BlockRequest) Serialize(ctx serde.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, req)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// BlockResponse is the response to a block request.
//
// - implements serde.Message
type BlockResponse struct {
	block SkipBlock
}

func NewBlockResponse(block SkipBlock) BlockResponse {
	return BlockResponse{
		block: block,
	}
}

func (resp BlockResponse) GetBlock() SkipBlock {
	return resp.block
}

// Serialize implements serde.Message.
func (resp BlockResponse) Serialize(ctx serde.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, resp)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type MessageFactory struct {
	payloadFactory serde.Factory
}

func NewMessageFactory(pf serde.Factory) MessageFactory {
	return MessageFactory{
		payloadFactory: pf,
	}
}

func (f MessageFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := requestFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, PayloadKey{}, f.payloadFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
