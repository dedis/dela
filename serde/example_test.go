package serde_test

import (
	"fmt"

	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
)

func ExampleMessage_Serialize_json() {
	ctx := json.NewContext()

	msg := exampleMessage{Value: 42}

	data, err := msg.Serialize(ctx)
	if err != nil {
		panic("serialization failed: " + err.Error())
	}

	fmt.Println(string(data))

	// Output: {"Value":42}
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

	// Output: {Value:42}
}

type exampleMessage struct {
	Value int
}

func (msg exampleMessage) Serialize(ctx serde.Context) ([]byte, error) {
	return ctx.Marshal(msg)
}

type exampleFactory struct{}

func (exampleFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	var m exampleMessage

	return m, ctx.Unmarshal(data, &m)
}
