package skipchain

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/dela/blockchain/skipchain/json"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/tmp"
)

// Blueprint is the proposition sent to create a new block.
type Blueprint struct {
	serde.UnimplementedMessage

	index    uint64
	previous Digest
	payload  proto.Message
}

// VisitJSON implements serde.Message.
func (b Blueprint) VisitJSON(serde.Serializer) (interface{}, error) {
	m := json.Blueprint{
		Index:    b.index,
		Previous: b.previous[:],
		Payload:  tmp.MarshalProto(b.payload),
	}

	return m, nil
}

// BlueprintFactory is a factory for the blueprint message.
//
// - implements serde.Factory
type BlueprintFactory struct {
	serde.UnimplementedFactory
}

// VisitJSON implements serde.Factory.
func (f BlueprintFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Blueprint{}
	err := in.Feed(&m)
	if err != nil {
		return nil, err
	}

	previous := Digest{}
	copy(previous[:], m.Previous)

	b := Blueprint{
		index:    m.Index,
		previous: previous,
		payload:  tmp.UnmarshalProto(m.Payload),
	}

	return b, nil
}
