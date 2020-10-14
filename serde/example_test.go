package serde_test

import (
	"errors"
	"fmt"

	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"go.dedis.ch/dela/serde/registry"
)

func ExampleMessage_Serialize_json() {
	// Register a JSON format engine for the message type.
	exampleRegistry.Register(serde.FormatJSON, exampleJSONFormat{})

	msg := exampleMessage{
		value: 42,
	}

	ctx := json.NewContext()

	data, err := msg.Serialize(ctx)
	if err != nil {
		panic("serialization failed: " + err.Error())
	}

	fmt.Println(string(data))

	// Output: {"value":42}
}

func ExampleFactory_Deserialize_json() {
	ctx := json.NewContext()

	data := []byte(`{"Value":42}`)

	factory := exampleFactory{}

	msg, err := factory.Deserialize(ctx, data)
	if err != nil {
		panic("deserialization failed: " + err.Error())
	}

	fmt.Printf("%+v", msg)

	// Output: {value:42}
}

var exampleRegistry = registry.NewSimpleRegistry()

// exampleMessage is the data model for a message example.
//
// - implements serde.Message
type exampleMessage struct {
	value int
}

// exampleFactory is a example of a message factory.
//
// - implements serde.Factory
type exampleFactory struct{}

// Deserialize implements serde.Factory. It populates the example message.
func (exampleFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := exampleRegistry.Get(ctx.GetFormat())

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	m, ok := msg.(exampleMessage)
	if !ok {
		return nil, errors.New("invalid message")
	}

	return m, nil
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
	var m exampleMessageJSON
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	msg := exampleMessage{
		value: m.Value,
	}

	return msg, nil
}
