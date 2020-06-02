# Serde

Serde is a serialization/deserialization abstraction. It has the purpose to
provide a simple API to serialize or deserialize messages without the concern of
the format, which is configurable.

Each message implementation is responsible to provide the correct data structure
for the encoding it wants to support.

## Serializer

The serializer expects a message interface as input when serializing. Basically,
the data model should implement this interface. It will provide the information
about the message structure for every supported encoding.

```go
type block struct {
    // Provides a default implementation for non-supported encodings.
    serde.UnimplementedMessage
}

type (b block) VisitJSON() (interface{}, error) {
    // toJSON populates the a block message compatible with the JSON format.
    return b.toJSON(), nil
}
```

Serializing a message implementation will produce the byte slice to be
transmitted over the communication channel.

```go
package jtypes

type BlockMessage {
    Index uint64
}
```

```go
package blockchain

ser := json.NewSerializer()
data, err := ser.Serialize(block{index: 42})
if err != nil {
    // do..
}
```

When a distant party receives the byte slice, it will decode using a factory.
The factory interface is similar to the message except that it provides the
correct interface to decode and then instantiate the message implementation.

```go
package blockchain

type blockFactory struct{
    serde.UnimplementedFactory
}

func (f blockFactory) VisitJSON(d serde.Deserializer) (serde.Message, error) {
    m := jtypes.BlockMessage{}
    err := d.Deserialize(&m)
    if err != nil {
        // do..
    }

    // fromJSON populates the block with the json message.
    return block{index: m.Index}, nil
}

ser := json.NewSerializer()
m, err := ser.Deserialize(data, blockFactory{})
if err != nil {
    // do..
}

block := m.(block)
```
