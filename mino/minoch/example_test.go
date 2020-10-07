package minoch

import (
	"context"
	"fmt"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

func ExampleRPC_Call() {
	manager := NewManager()

	minoA := MustCreate(manager, "A")
	minoB := MustCreate(manager, "B")

	roster := mino.NewAddresses(minoA.GetAddress(), minoB.GetAddress())

	nsA, err := minoA.MakeNamespace("example")
	if err != nil {
		panic("namespace failed: " + err.Error())
	}

	rpcA, err := nsA.MakeRPC("hello", exampleHandler{}, exampleFactory{})
	if err != nil {
		panic("rpc failed: " + err.Error())
	}

	nsB, err := minoB.MakeNamespace("example")
	if err != nil {
		panic("namespace failed: " + err.Error())
	}

	_, err = nsB.MakeRPC("hello", exampleHandler{}, exampleFactory{})
	if err != nil {
		panic("rpc failed: " + err.Error())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msg := exampleMessage{value: "Hello World!"}

	resps, err := rpcA.Call(ctx, msg, roster)
	if err != nil {
		panic("call failed: " + err.Error())
	}

	for resp := range resps {
		reply, err := resp.GetMessageOrError()
		if err != nil {
			panic("error in response: " + err.Error())
		}

		fmt.Println(reply.(exampleMessage).value)
	}

	// Output: Hello World!
	// Hello World!
}

// exampleHandler is an RPC handler example.
//
// - implements mino.Handler
type exampleHandler struct {
	mino.UnsupportedHandler
}

// Process implements mino.Handler. It returns the message received.
func (exampleHandler) Process(req mino.Request) (serde.Message, error) {
	return req.Message, nil
}

// exampleMessage is an example of a message.
//
// - implements serde.Message
type exampleMessage struct {
	value string
}

// Serialize implements serde.Message. It returns the value contained in the
// message.
func (m exampleMessage) Serialize(serde.Context) ([]byte, error) {
	return []byte(m.value), nil
}

// exampleFactory is an example of a factory.
//
// - implements serde.Factory
type exampleFactory struct{}

// Deserialize implements serde.Factory. It returns the message using data as
// the inner value.
func (exampleFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return exampleMessage{value: string(data)}, nil
}
