// Package serde defines the primitives to serialize and deserialize (serde)
// network messages.
package serde

import "golang.org/x/xerrors"

// RawMessage is an interface to define raw messages inside another message.
type RawMessage []byte

// MarshalJSON implements json.Marshaler.
func (raw RawMessage) MarshalJSON() ([]byte, error) {
	return raw, nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (raw *RawMessage) UnmarshalJSON(data []byte) error {
	if raw == nil {
		return xerrors.New("oops")
	}
	*raw = append((*raw)[0:0], data...)
	return nil
}

// Encoder is an interface to serialize and deserialize messages. It offers an
// API to automatically detect the type of a message.
type Encoder interface {
	// Encode takes a messages and returns its serialized form as a buffer. An
	// error is returned if the serialization fails.
	Encode(m interface{}) ([]byte, error)

	// Decode takes a buffer and a message. It deserializes the buffer into the
	// message and returns an error if it fails.
	Decode(buffer []byte, m interface{}) error

	// Wrap takes a message and returns its serialized form as a buffer. It has
	// the particularity to be unwrapped without providing an implementation of
	// the message. A message must be registered to be correctly unwrapped.
	Wrap(m interface{}) ([]byte, error)

	// Unwrap takes a buffer and returns the message deserialized, or an error
	// if it cannot. The message must be registered to be unwrapped.
	Unwrap(buffer []byte) (interface{}, error)

	// Raw produces the RawMessage for a given message.
	Raw(m interface{}) (RawMessage, error)

	// Unraw takes a RawMessage and returns the message deserialized.
	Unraw(r RawMessage) (interface{}, error)
}
