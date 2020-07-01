package types

import (
	"go.dedis.ch/dela/serdeng"
	"go.dedis.ch/dela/serdeng/registry"
)

var (
	bpFormats      = registry.NewSimpleRegistry()
	requestFormats = registry.NewSimpleRegistry()
)

func RegisterBlueprintFormats(c serdeng.Codec, f serdeng.Format) {
	bpFormats.Register(c, f)
}

func RegisterRequestFormats(c serdeng.Codec, f serdeng.Format) {
	requestFormats.Register(c, f)
}

// Blueprint is the proposition sent to create a new block.
//
// - implements serde.Message
type Blueprint struct {
	index    uint64
	previous Digest
	data     serdeng.Message
}

func NewBlueprint(index uint64, previous []byte, data serdeng.Message) Blueprint {
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

func (b Blueprint) GetData() serdeng.Message {
	return b.data
}

// Serialize implements serde.Message.
func (b Blueprint) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := bpFormats.Get(ctx.GetName())

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
	factory serdeng.Factory
}

func NewBlueprintFactory(f serdeng.Factory) BlueprintFactory {
	return BlueprintFactory{
		factory: f,
	}
}

// Deserialize implements serde.Factory.
func (f BlueprintFactory) Deserialize(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	format := bpFormats.Get(ctx.GetName())

	ctx = serdeng.WithFactory(ctx, DataKey{}, f.factory)

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
func (p PropagateGenesis) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetName())

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
func (req BlockRequest) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetName())

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
func (resp BlockResponse) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, resp)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type MessageFactory struct {
	payloadFactory serdeng.Factory
}

func NewMessageFactory(pf serdeng.Factory) MessageFactory {
	return MessageFactory{
		payloadFactory: pf,
	}
}

func (f MessageFactory) Deserialize(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	format := requestFormats.Get(ctx.GetName())

	ctx = serdeng.WithFactory(ctx, PayloadKey{}, f.payloadFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
