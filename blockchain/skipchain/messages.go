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
		return nil, xerrors.Errorf("couldn't serialize payload: %v", err)
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
		return nil, xerrors.Errorf("couldn't deserialize payload: %v", err)
	}

	b := Blueprint{
		index:    m.Index,
		previous: previous,
		data:     data,
	}

	return b, nil
}

// PropagateGenesis is the message sent to share the genesis block.
//
// - implements serde.Message
type PropagateGenesis struct {
	serde.UnimplementedMessage

	genesis SkipBlock
}

// VisitJSON implements serde.Message. It serializes the propagate message in
// JSON format.
func (p PropagateGenesis) VisitJSON(ser serde.Serializer) (interface{}, error) {
	block, err := ser.Serialize(p.genesis)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize genesis: %v", err)
	}

	m := json.PropagateGenesis{
		Genesis: block,
	}

	return json.Message{Propagate: &m}, nil
}

// BlockRequest is the message sent to request a block.
//
// - implements serde.Message
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

	return json.Message{Request: &m}, nil
}

// BlockResponse is the response to a block request.
//
// - implements serde.Message
type BlockResponse struct {
	serde.UnimplementedMessage

	block SkipBlock
}

// VisitJSON implements serde.Message. It serializes the block response in JSON
// format.
func (resp BlockResponse) VisitJSON(ser serde.Serializer) (interface{}, error) {
	block, err := ser.Serialize(resp.block)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize block: %v", err)
	}

	m := json.BlockResponse{
		Block: block,
	}

	return json.Message{Response: &m}, nil
}

// MessageFactory is a message factory for the skipchain messages.
//
// - implements serde.Factory
type MessageFactory struct {
	serde.UnimplementedFactory

	blockFactory serde.Factory
}

// VisitJSON implements serde.Factory. It deserializes the messages in JSON
// format.
func (f MessageFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Message{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	if m.Propagate != nil {
		var genesis SkipBlock
		err = in.GetSerializer().Deserialize(m.Propagate.Genesis, f.blockFactory, &genesis)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize genesis: %v", err)
		}

		p := PropagateGenesis{
			genesis: genesis,
		}

		return p, nil
	}

	if m.Request != nil {
		req := BlockRequest{
			from: m.Request.From,
			to:   m.Request.To,
		}

		return req, nil
	}

	if m.Response != nil {
		var block SkipBlock
		err = in.GetSerializer().Deserialize(m.Response.Block, f.blockFactory, &block)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize block: %v", err)
		}

		resp := BlockResponse{
			block: block,
		}

		return resp, nil
	}

	return nil, xerrors.New("message is empty")
}
