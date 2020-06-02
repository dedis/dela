// Package serde defines the primitives to serialize and deserialize (serde)
// network messages.
//
// The format can be chosen among three options:
// - JSON
// - Gob
// - Protobuf
package serde

// Message is the interface a data model should implemented to be serialized and
// deserialized.
type Message interface {
	VisitJSON() (interface{}, error)

	VisitGob() (interface{}, error)

	VisitProto() (interface{}, error)
}

// Deserializer is the interface provided to a factory to deserialze raw
// messages.
type Deserializer interface {
	Deserialize(interface{}) error
}

// Factory is the interface to implement to instantiate a data model from the
// raw message.
type Factory interface {
	VisitJSON(Deserializer) (Message, error)

	VisitGob(Deserializer) (Message, error)

	VisitProto(Deserializer) (Message, error)
}

// Store registers the factory of a given message implementation.
type Store interface {
	Get(key string) Factory

	Add(m Message, f Factory) error

	KeyOf(m Message) string
}

// Serializer is an interface that provides promitives to serialize and
// deserialize a data model.
type Serializer interface {
	GetStore() Store

	Serialize(Message) ([]byte, error)

	Wrap(Message) ([]byte, error)

	Deserialize([]byte, Factory) (Message, error)

	Unwrap([]byte) (Message, error)
}
