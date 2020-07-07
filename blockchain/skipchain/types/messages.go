package types

import (
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var (
	bpFormats      = registry.NewSimpleRegistry()
	requestFormats = registry.NewSimpleRegistry()
)

// RegisterBlueprintFormat registers the engine for the provided format.
func RegisterBlueprintFormat(c serde.Format, f serde.FormatEngine) {
	bpFormats.Register(c, f)
}

// RegisterRequestFormat registers the engine for the provided format.
func RegisterRequestFormat(c serde.Format, f serde.FormatEngine) {
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

// NewBlueprint creates a new blueprint.
func NewBlueprint(index uint64, previous []byte, data serde.Message) Blueprint {
	bp := Blueprint{
		index: index,
		data:  data,
	}

	copy(bp.previous[:], previous)

	return bp
}

// GetIndex returns the index of the blueprint.
func (b Blueprint) GetIndex() uint64 {
	return b.index
}

// GetPrevious returns the identifier of the previous block.
func (b Blueprint) GetPrevious() []byte {
	return b.previous[:]
}

// GetData returns the data of the blueprint.
func (b Blueprint) GetData() serde.Message {
	return b.data
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data if appropriate, otherwise it returns nil.
func (b Blueprint) Serialize(ctx serde.Context) ([]byte, error) {
	format := bpFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, b)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode blueprint: %v", err)
	}

	return data, nil
}

// DataKeyFac is the key of the data factory.
type DataKeyFac struct{}

// BlueprintFactory is a factory for the blueprint message.
//
// - implements serde.Factory
type BlueprintFactory struct {
	factory serde.Factory
}

// NewBlueprintFactory creates a new blueprint factory.
func NewBlueprintFactory(f serde.Factory) BlueprintFactory {
	return BlueprintFactory{
		factory: f,
	}
}

// Deserialize implements serde.Factory. It looks up the format and returns the
// blueprint deserialized if apprpriate, otherwise an error.
func (f BlueprintFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := bpFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, DataKeyFac{}, f.factory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode blueprint: %v", err)
	}

	return msg, nil
}

// PropagateGenesis is the message sent to share the genesis block.
//
// - implements serde.Message
type PropagateGenesis struct {
	genesis SkipBlock
}

// NewPropagateGenesis creates a new genesis propagation request.
func NewPropagateGenesis(genesis SkipBlock) PropagateGenesis {
	return PropagateGenesis{
		genesis: genesis,
	}
}

// GetGenesis returns the genesis block.
func (p PropagateGenesis) GetGenesis() SkipBlock {
	return p.genesis
}

// Serialize implements serde.Message.
func (p PropagateGenesis) Serialize(ctx serde.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, p)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode request: %v", err)
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

// NewBlockRequest creates a new block request.
func NewBlockRequest(from, to uint64) BlockRequest {
	return BlockRequest{
		from: from,
		to:   to,
	}
}

// GetFrom returns the index of the first block to request.
func (req BlockRequest) GetFrom() uint64 {
	return req.from
}

// GetTo returns the index of the latest known block.
func (req BlockRequest) GetTo() uint64 {
	return req.to
}

// Serialize implements serde.Message. It looks up the format and returns the
// request serialized if appropriate, otherwise an error.
func (req BlockRequest) Serialize(ctx serde.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, req)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode request: %v", err)
	}

	return data, nil
}

// BlockResponse is the response to a block request.
//
// - implements serde.Message
type BlockResponse struct {
	block SkipBlock
}

// NewBlockResponse creates a new block response.
func NewBlockResponse(block SkipBlock) BlockResponse {
	return BlockResponse{
		block: block,
	}
}

// GetBlock returns the block.
func (resp BlockResponse) GetBlock() SkipBlock {
	return resp.block
}

// Serialize implements serde.Message.
func (resp BlockResponse) Serialize(ctx serde.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, resp)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode response: %v", err)
	}

	return data, nil
}

// RequestFactory is the factory to deserialize messages.
//
// - implements serde.Factory
type RequestFactory struct {
	payloadFactory serde.Factory
}

// NewRequestFactory creates a new message factory.
func NewRequestFactory(pf serde.Factory) RequestFactory {
	return RequestFactory{
		payloadFactory: pf,
	}
}

// Deserialize implements serde.Factory. It looks up the format and populates
// the message with the data if appropriate, otherwise it returns an error.
func (f RequestFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := requestFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, PayloadKeyFac{}, f.payloadFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode request: %v", err)
	}

	return msg, nil
}
