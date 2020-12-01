package minogrpc

import (
	"context"
	"fmt"
	"time"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router/tree"
	"go.dedis.ch/dela/serde"
)

func ExampleRPC_Call() {
	mA, err := NewMinogrpc(ParseAddress("127.0.0.1", 0), tree.NewRouter(NewAddressFactory()))
	if err != nil {
		panic("overlay A failed: " + err.Error())
	}

	rpcA := mino.MustCreateRPC(mA, "test", exampleHandler{}, exampleFactory{})

	mB, err := NewMinogrpc(ParseAddress("127.0.0.1", 0), tree.NewRouter(NewAddressFactory()))
	if err != nil {
		panic("overlay B failed: " + err.Error())
	}

	mino.MustCreateRPC(mB, "test", exampleHandler{}, exampleFactory{})

	mA.GetCertificateStore().Store(mB.GetAddress(), mB.GetCertificate())
	mB.GetCertificateStore().Store(mA.GetAddress(), mA.GetCertificate())

	addrs := mino.NewAddresses(mA.GetAddress(), mB.GetAddress())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resps, err := rpcA.Call(ctx, exampleMessage{value: "Hello World!"}, addrs)
	if err != nil {
		panic("call failed: " + err.Error())
	}

	for resp := range resps {
		reply, err := resp.GetMessageOrError()
		if err != nil {
			panic("error in reply: " + err.Error())
		}

		if resp.GetFrom().Equal(mA.GetAddress()) {
			fmt.Println("A", reply.(exampleMessage).value)
		}
		if resp.GetFrom().Equal(mB.GetAddress()) {
			fmt.Println("B", reply.(exampleMessage).value)
		}
	}

	// Unordered output: A Hello World!
	// B Hello World!
}

func ExampleRPC_Stream() {
	mA, err := NewMinogrpc(ParseAddress("127.0.0.1", 20000), tree.NewRouter(NewAddressFactory()))
	if err != nil {
		panic("overlay A failed: " + err.Error())
	}

	rpcA := mino.MustCreateRPC(mA, "test", exampleHandler{}, exampleFactory{})

	mB, err := NewMinogrpc(ParseAddress("127.0.0.1", 30000), tree.NewRouter(NewAddressFactory()))
	if err != nil {
		panic("overlay B failed: " + err.Error())
	}

	mino.MustCreateRPC(mB, "test", exampleHandler{}, exampleFactory{})

	mA.GetCertificateStore().Store(mB.GetAddress(), mB.GetCertificate())
	mB.GetCertificateStore().Store(mA.GetAddress(), mA.GetCertificate())

	addrs := mino.NewAddresses(mA.GetAddress(), mB.GetAddress())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, recv, err := rpcA.Stream(ctx, addrs)
	if err != nil {
		panic("stream failed: " + err.Error())
	}

	err = <-sender.Send(exampleMessage{value: "Hello World!"}, mB.GetAddress())
	if err != nil {
		panic("failed to send: " + err.Error())
	}

	from, msg, err := recv.Recv(ctx)
	if err != nil {
		panic("failed to receive: " + err.Error())
	}

	if from.Equal(mB.GetAddress()) {
		fmt.Println("B", msg.(exampleMessage).value)
	}

	// Output: B Hello World!
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

// Stream implements mino.Handler. It returns the message to the sender.
func (exampleHandler) Stream(sender mino.Sender, recv mino.Receiver) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	from, msg, err := recv.Recv(ctx)
	if err != nil {
		return err
	}

	err = <-sender.Send(msg, from)
	if err != nil {
		return err
	}

	return nil
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
