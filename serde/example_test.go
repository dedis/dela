package serde_test

import (
	"errors"
	"fmt"

	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"go.dedis.ch/dela/serde/registry"
	"go.dedis.ch/dela/serde/xml"
)

func ExampleMessage_Serialize() {
	// Register a JSON format engine for the message type.
	exampleRegistry.Register(serde.FormatJSON, exampleJSONFormat{})
	// Register a Gob format engine for the message type.
	exampleRegistry.Register(serde.FormatXML, exampleXMLFormat{})

	msg := exampleMessage{
		value: 42,
	}

	data, err := msg.Serialize(json.NewContext())
	if err != nil {
		panic("serialization failed: " + err.Error())
	}

	fmt.Println("JSON", string(data))

	data, err = msg.Serialize(xml.NewContext())
	if err != nil {
		panic("serialization failed: " + err.Error())
	}

	fmt.Println("XML", string(data))

	// Output: JSON {"value":42}
	// XML <exampleMessageXML><value>42</value></exampleMessageXML>
}

func ExampleFactory_Deserialize() {
	factory := exampleFactory{}

	msg, err := factory.Deserialize(json.NewContext(), []byte(`{"Value":42}`))
	if err != nil {
		panic("deserialization failed: " + err.Error())
	}

	fmt.Printf("%+v\n", msg)

	msg, err = factory.Deserialize(xml.NewContext(), []byte("<exampleMessageXML><value>12</value></exampleMessageXML>"))
	if err != nil {
		panic("deserialization failed: " + err.Error())
	}

	fmt.Printf("%+v", msg)

	// Output: {value:42}
	// {value:12}
}

var exampleRegistry = registry.NewSimpleRegistry()

// exampleMessage is the data model for a message example.
//
// - implements serde.Message
type exampleMessage struct {
	value int
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

// Decode implements serde.FormatEngine. It populates a message example if
// appropritate, otherwise it returns an error.
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

// exampleMessageXML is an XML message for a message example.
type exampleMessageXML struct {
	Value int `xml:"value"`
}

// exampleXMLFormat is an example if a format to serialize a message example
// using the XML encoding.
//
// - implements serde.FormatEngine
type exampleXMLFormat struct{}

// Encode implements serde.FormatEngine. It formats the message to comply with
// the XML encoding and marshal it.
func (exampleXMLFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	example, ok := msg.(exampleMessage)
	if !ok {
		return nil, errors.New("unsupported message")
	}

	m := exampleMessageXML{
		Value: example.value,
	}

	return ctx.Marshal(m)
}

// Decode implements serde.FormatEngine. It populates a message example if
// appropritate, otherwise it returns an error.
func (exampleXMLFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	var m exampleMessageXML
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	msg := exampleMessage{
		value: m.Value,
	}

	return msg, nil
}
