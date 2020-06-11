package skipchain

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/dela/blockchain/skipchain/json"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/tmp"
	"golang.org/x/xerrors"
)

// Blueprint is the proposition sent to create a new block.
//
// - implements serde.Message
type Blueprint struct {
	serde.UnimplementedMessage

	index    uint64
	previous Digest
	payload  proto.Message
}

// VisitJSON implements serde.Message. It serializes the blueprint in JSON
// format.
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

	b := Blueprint{
		index:    m.Index,
		previous: previous,
		payload:  tmp.UnmarshalProto(m.Payload),
	}

	return b, nil
}
