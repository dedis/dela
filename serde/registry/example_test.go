package registry_test

import (
	"errors"
	"fmt"

	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"go.dedis.ch/dela/serde/registry"
)

func ExampleRegistry_Register() {
	exampleRegistry.Register(serde.FormatJSON, exampleJSONFormat{})

	msg := exampleMessage{
		value: 42,
	}

	data, err := msg.Serialize(json.NewContext())
	if err != nil {
		panic("serialization failed: " + err.Error())
	}

	fmt.Println(string(data))

	// Output: {"value":42}
}

var exampleRegistry = registry.NewSimpleRegistry()

// exampleMessage is the data model for a message example.
//
// - implements serde.Message
type exampleMessage struct {
	value int
}

// exampleMessageJSON is a JSON message for a message example.
type exampleMessageJSON struct {
	Value int `json:"value"`
}

// Serialize implements serde.Message. It returns the JSON serialization of a
// message example.
func (m exampleMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := exampleRegistry.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// exampleJSONFormat is an example of a format to serialize a message example
// using a JSON encoding.
//
// - implements serde.FormatEngine
type exampleJSONFormat struct{}

// Encode implements serde.FormatEngine. It populates a message that complies
// the JSON encoding and marshal it.
func (exampleJSONFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	example, ok := msg.(exampleMessage)
	if !ok {
		return nil, errors.New("unsupported message")
	}

	m := exampleMessageJSON{
		Value: example.value,
	}

	return ctx.Marshal(m)
}

// Decode implements serde.FormatEngine. It is not implemented in this example.
func (exampleJSONFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	return nil, errors.New("not implemented")
}
