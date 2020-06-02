// Package serde defines the primitives to serialize and deserialize (serde)
// network messages.
package serde

import "golang.org/x/xerrors"

// Raw is an interface to define raw messages inside another message.
type Raw []byte

// MarshalJSON implements json.Marshaler.
func (m Raw) MarshalJSON() ([]byte, error) {
	return m, nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *Raw) UnmarshalJSON(data []byte) error {
	if m == nil {
		return xerrors.New("oops")
	}
	*m = append((*m)[0:0], data...)
	return nil
}

// Packable is an interface to define how an object should pack itself to a
// network message.
type Packable interface {
	Pack(Encoder) (interface{}, error)
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
	Wrap(m interface{}) (Raw, error)

	// Unwrap takes a buffer and returns the message deserialized, or an error
	// if it cannot. The message must be registered to be unwrapped.
	Unwrap(m Raw) (interface{}, error)

	// MessageOf returns the network message of the packable object.
	MessageOf(Packable) (interface{}, error)
}
