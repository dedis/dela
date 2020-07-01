package serde

import "io"

type Codec string

const (
	CodecJSON     Codec = "JSON"
	CodecGob      Codec = "Gob"
	CodecProtobuf Codec = "Protobuf"
)

type Message interface {
	Serialize(Context) ([]byte, error)
}

type Factory interface {
	Deserialize(Context, []byte) (Message, error)
}

type Format interface {
	Encode(Context, Message) ([]byte, error)
	Decode(Context, []byte) (Message, error)
}

type Fingerprinter interface {
	Fingerprint(io.Writer) error
}
