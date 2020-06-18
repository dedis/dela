package json

import (
	"encoding/json"

	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/blockchain/skipchain/types"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterBlockFormat(serdeng.CodecJSON, blockFormat{})
	types.RegisterVerifiableBlockFormats(serdeng.CodecJSON, newVerifiableFormat())
	types.RegisterBlueprintFormats(serdeng.CodecJSON, blueprintFormat{})
	types.RegisterRequestFormats(serdeng.CodecJSON, newRequestFormat())
}

// Blueprint is a JSON message to send a proposal.
type Blueprint struct {
	Index    uint64
	Previous []byte
	Payload  []byte
}

// SkipBlock is the JSON message for a block.
type SkipBlock struct {
	Index     uint64
	GenesisID []byte
	Backlink  []byte
	Payload   json.RawMessage
}

// VerifiableBlock is the JSON message for a verifiable block.
type VerifiableBlock struct {
	Block json.RawMessage
	Chain json.RawMessage
}

// PropagateGenesis the the JSON message to share a genesis block.
type PropagateGenesis struct {
	Genesis json.RawMessage
}

// BlockRequest is the JSON message to request a chain of blocks.
type BlockRequest struct {
	From uint64
	To   uint64
}

// BlockResponse is the response of a block request.
type BlockResponse struct {
	Block json.RawMessage
}

type Message struct {
	Propagate *PropagateGenesis `json:",omitempty"`
	Request   *BlockRequest     `json:",omitempty"`
	Response  *BlockResponse    `json:",omitempty"`
}

type blockFormat struct {
	hashFactory crypto.HashFactory
}

func (f blockFormat) Encode(ctx serdeng.Context, msg serdeng.Message) ([]byte, error) {
	block, ok := msg.(types.SkipBlock)
	if !ok {
		return nil, xerrors.New("invalid block message")
	}

	payload, err := block.Payload.Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize payload: %v", err)
	}

	m := SkipBlock{
		Index:     block.Index,
		GenesisID: block.GenesisID.Bytes(),
		Backlink:  block.BackLink.Bytes(),
		Payload:   payload,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (f blockFormat) Decode(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	m := SkipBlock{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	factory := ctx.GetFactory(types.PayloadKey{})
	if factory == nil {
		return nil, xerrors.New("payload factory is missing")
	}

	msg, err := factory.Deserialize(ctx, m.Payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize payload: %v", err)
	}

	payload, ok := msg.(blockchain.Payload)
	if !ok {
		return nil, xerrors.Errorf("invalid payload of type '%T'", msg)
	}

	opts := []types.SkipBlockOption{
		types.WithIndex(m.Index),
		types.WithGenesisID(m.GenesisID),
		types.WithBackLink(m.Backlink),
		types.WithPayload(payload),
	}

	if f.hashFactory != nil {
		// Keep the skipblock default factory unless it is effectively set.
		opts = append(opts, types.WithHashFactory(f.hashFactory))
	}

	block, err := types.NewSkipBlock(opts...)

	if err != nil {
		return nil, xerrors.Errorf("couldn't create block: %v", err)
	}

	return block, nil
}

type verifiableFormat struct {
	blockFormat serdeng.Format
}

func newVerifiableFormat() verifiableFormat {
	return verifiableFormat{
		blockFormat: blockFormat{},
	}
}

func (f verifiableFormat) Encode(ctx serdeng.Context, msg serdeng.Message) ([]byte, error) {
	vb, ok := msg.(types.VerifiableBlock)
	if !ok {
		return nil, xerrors.New("invalid block message")
	}

	block, err := f.blockFormat.Encode(ctx, vb.SkipBlock)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize block: %v", err)
	}

	chain, err := vb.Chain.Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize chain: %v", err)
	}

	m := VerifiableBlock{
		Block: block,
		Chain: chain,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (f verifiableFormat) Decode(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	m := VerifiableBlock{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	chain, err := decodeChain(ctx, m.Chain)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize chain: %v", err)
	}

	block, err := f.decodeBlock(ctx, m.Block)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize block: %v", err)
	}

	vb := types.VerifiableBlock{
		SkipBlock: block,
		Chain:     chain,
	}

	return vb, nil
}

func decodeChain(ctx serdeng.Context, data []byte) (consensus.Chain, error) {
	factory := ctx.GetFactory(types.ChainKey{})

	fac, ok := factory.(consensus.ChainFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory of type '%T'", factory)
	}

	chain, err := fac.ChainOf(ctx, data)
	if err != nil {
		return nil, err
	}

	return chain, nil
}

func (f verifiableFormat) decodeBlock(ctx serdeng.Context, data []byte) (types.SkipBlock, error) {
	msg, err := f.blockFormat.Decode(ctx, data)
	if err != nil {
		return types.SkipBlock{}, err
	}

	return msg.(types.SkipBlock), nil
}

type blueprintFormat struct{}

func (f blueprintFormat) Encode(ctx serdeng.Context, msg serdeng.Message) ([]byte, error) {
	bp, ok := msg.(types.Blueprint)
	if !ok {
		return nil, xerrors.New("invalid blueprint message")
	}

	payload, err := bp.GetData().Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize payload: %v", err)
	}

	m := Blueprint{
		Index:    bp.GetIndex(),
		Previous: bp.GetPrevious(),
		Payload:  payload,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (f blueprintFormat) Decode(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	m := Blueprint{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize blueprint: %v", err)
	}

	factory := ctx.GetFactory(types.DataKey{})
	if factory == nil {
		return nil, xerrors.New("missing data factory")
	}

	payload, err := factory.Deserialize(ctx, m.Payload)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize payload: %v", err)
	}

	b := types.NewBlueprint(m.Index, m.Previous, payload)

	return b, nil
}

type requestFormat struct {
	blockFormat serdeng.Format
}

func newRequestFormat() requestFormat {
	return requestFormat{
		blockFormat: blockFormat{},
	}
}

func (f requestFormat) Encode(ctx serdeng.Context, msg serdeng.Message) ([]byte, error) {
	var req Message

	switch in := msg.(type) {
	case types.PropagateGenesis:
		block, err := f.blockFormat.Encode(ctx, in.GetGenesis())
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize genesis: %v", err)
		}

		m := PropagateGenesis{
			Genesis: block,
		}

		req = Message{Propagate: &m}
	case types.BlockRequest:
		m := BlockRequest{
			From: in.GetFrom(),
			To:   in.GetTo(),
		}

		req = Message{Request: &m}
	case types.BlockResponse:
		block, err := f.blockFormat.Encode(ctx, in.GetBlock())
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize block: %v", err)
		}

		m := BlockResponse{
			Block: block,
		}

		req = Message{Response: &m}
	default:
		return nil, xerrors.New("invalid message")
	}

	data, err := ctx.Marshal(req)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (f requestFormat) Decode(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	m := Message{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	if m.Propagate != nil {
		genesis, err := f.blockFormat.Decode(ctx, m.Propagate.Genesis)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize genesis: %v", err)
		}

		p := types.NewPropagateGenesis(genesis.(types.SkipBlock))

		return p, nil
	}

	if m.Request != nil {
		req := types.NewBlockRequest(m.Request.From, m.Request.To)

		return req, nil
	}

	if m.Response != nil {
		block, err := f.blockFormat.Decode(ctx, m.Response.Block)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize block: %v", err)
		}

		resp := types.NewBlockResponse(block.(types.SkipBlock))

		return resp, nil
	}

	return nil, xerrors.New("message is empty")
}
