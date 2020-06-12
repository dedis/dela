package skipchain

import (
	"go.dedis.ch/dela/blockchain/skipchain/json"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// Blueprint is the proposition sent to create a new block.
//
// - implements serde.Message
type Blueprint struct {
	serde.UnimplementedMessage

	index    uint64
	previous Digest
	data     serde.Message
}

// VisitJSON implements serde.Message. It serializes the blueprint in JSON
// format.
func (b Blueprint) VisitJSON(ser serde.Serializer) (interface{}, error) {
	payload, err := ser.Serialize(b.data)
	if err != nil {
		return nil, err
	}

	m := json.Blueprint{
		Index:    b.index,
		Previous: b.previous[:],
		Payload:  payload,
	}

	return m, nil
}

// BlueprintFactory is a factory for the blueprint message.
//
// - implements serde.Factory
type BlueprintFactory struct {
	serde.UnimplementedFactory

	factory serde.Factory
}

// VisitJSON implements serde.Factory. It deserializes the blueprint in JSON
// format.
func (f BlueprintFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Blueprint{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize blueprint: %v", err)
	}

	previous := Digest{}
	copy(previous[:], m.Previous)

	var data serde.Message
	err = in.GetSerializer().Deserialize(m.Payload, f.factory, &data)
	if err != nil {
		return nil, err
	}

	b := Blueprint{
		index:    m.Index,
		previous: previous,
		data:     data,
	}

	return b, nil
}

// PropagateGenesis is the message sent to share the genesis block.
type PropagateGenesis struct {
	serde.UnimplementedMessage

	genesis SkipBlock
}

// VisitJSON implements serde.Message. It serializes the propagate message in
// JSON format.
func (p PropagateGenesis) VisitJSON(ser serde.Serializer) (interface{}, error) {
	block, err := ser.Serialize(p.genesis)
	if err != nil {
		return nil, err
	}

	m := json.PropagateGenesis{
		Genesis: block,
	}

	return m, nil
}

type propagateFactory struct {
	serde.UnimplementedFactory

	blockFactory serde.Factory
}

func (f propagateFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.PropagateGenesis{}
	err := in.Feed(&m)
	if err != nil {
		return nil, err
	}

	var genesis SkipBlock
	err = in.GetSerializer().Deserialize(m.Genesis, f.blockFactory, &genesis)
	if err != nil {
		return nil, err
	}

	p := PropagateGenesis{
		genesis: genesis,
	}

	return p, nil
}

// BlockRequest is the message sent to request a block.
type BlockRequest struct {
	serde.UnimplementedMessage

	from uint64
	to   uint64
}

// VisitJSON implements serde.Message. It serializes the block request in JSON
// format.
func (req BlockRequest) VisitJSON(ser serde.Serializer) (interface{}, error) {
	m := json.BlockRequest{
		From: req.from,
		To:   req.to,
	}

	return m, nil
}

type requestFactory struct {
	serde.UnimplementedFactory
}

func (f requestFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.BlockRequest{}
	err := in.Feed(&m)
	if err != nil {
		return nil, err
	}

	req := BlockRequest{
		from: m.From,
		to:   m.To,
	}

	return req, nil
}

// BlockResponse is the response to a block request.
type BlockResponse struct {
	serde.UnimplementedMessage

	block SkipBlock
}

// VisitJSON implements serde.Message. It serializes the block response in JSON
// format.
func (resp BlockResponse) VisitJSON(ser serde.Serializer) (interface{}, error) {
	block, err := ser.Serialize(resp.block)
	if err != nil {
		return nil, err
	}

	m := json.BlockResponse{
		Block: block,
	}

	return m, nil
}

type responseFactory struct {
	serde.UnimplementedFactory

	blockFactory serde.Factory
}

func (f responseFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.BlockResponse{}
	err := in.Feed(&m)
	if err != nil {
		return nil, err
	}

	var block SkipBlock
	err = in.GetSerializer().Deserialize(m.Block, f.blockFactory, &block)
	if err != nil {
		return nil, err
	}

	resp := BlockResponse{
		block: block,
	}

	return resp, nil
}
