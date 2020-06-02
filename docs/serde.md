# Serde strawman I

Serde is a serialization/deserialization abstraction. It has the purpose to
provide a simple API to encode or decode messages without the concern of the
format, which is configurable.

The messages are defined in Go and serde takes care of generating the message
schema when appropriate. This allows one to opt in or out from a supported
format.

## Encoder

When the type of the message is known by the two parties involved, the usage is
quite simple and use the common pattern in the different encoding library.

```go
encoder := json.NewEncoder()

buffer, err := encoder.Encode(&Example{})
checkError(err)

var ret Example{}
err = encoder.Decode(&ret)
checkError(err)
```

If the distant party wants to accept multiple type of messages and not relay on
a particular one, the message can be wrapped. The message needs to be registered
to be unwrapped on the other side.

```go
// Register the message so that it can be looked up when unwrapping.
serde.Register(Example{})

encoder := json.NewEncoder()

buffer, err := encoder.Wrap(&Example{})
checkError(err)

ret, err := encoder.Unwrap(buffer)
checkError(err)
```

Finally, it may happen that a message definition contains a generic message
which is only known at runtime. A dedicated type can be used on that purpose.

```go
encoder := json.NewEncoder()

type BlockMessage struct {
    Index   uint64
    Payload serde.Raw
}

payload, err := encoder.Wrap(Example{})
checkError(err)

msg := BlockMessage{
    Index: 0,
    Payload: payload,
}

buffer, err := encoder.Encode(msg)
checkError(err)
```

## Format optimization

Serde allows to bypass the reflection to speedup the message serialization and
deserialization. Each implementation will detect if the given message is
natively supported by the format and use the dedicated functions instead.

## Data Model vs Message

Dela differentiates the network messages and the data models.

```go
package block

type BlockMessage struct {
    Index uint64
}

type Block interface{
    GetIndex() uint64

    Pack(e serde.Encoder) (BlockMessage, error)
}

type BlockFactory interface {
    // FromMessage unwraps the message and returns the block associated.
    FromMessage(m serde.Message) (Block, error)
}
```

## TDB

- Check when registering the message that types are supported, or support
  anything and tries the best when decoding ?

## Serde strawman II

```go
package serde

type Message interface { // Visitor
    VisitJSON() (interface{}, error)
    VisitGob() (interface{}, error)
    VisitProto() (interface{}, error)
}

type Deserializer interface {
    Decode([]byte, interface{}) error
}

type Factory interface {
    // TBD: input parameters to allow decoding
    VisitJSON(d Deserializer, buffer []byte) (Message, error)
    VisitGob(d Deserializer, buffer []byte) (Message, error)
    VisitProto(d Deserializer, buffer []byte) (Message, error)
}

type Encoder interface {
    Encode(Message) ([]byte, error) // Accept
    Wrap(Message) ([]byte, error) // Accept with serde wrapper

    Decode([]byte, Factory) (Message, error)
    Unwrap([]byte) (Message, error)
}
```

```go
type Block interface{
    serde.Message

    GetIndex() uint64
}
```
