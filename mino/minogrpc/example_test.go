package minogrpc

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/mino/router/flat"
	"testing"
	"time"

	"go.dedis.ch/dela/internal/tracing"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router/tree"
	"go.dedis.ch/dela/serde"
)

func TestRpcStreamTree(t *testing.T) {
	NB_NODES := 200

	nodes := createNodes(NB_NODES, false)
	rpcs := createRpcs(nodes)
	exchangeCertificates(nodes)
	players := createPlayers(nodes)
	msgs := generateMessages(NB_NODES)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i, r := range rpcs {
		go func(i int, rpc mino.RPC) {
			sender, receiver, err := rpc.Stream(ctx, players)
			if err != nil {
				panic("stream failed: " + err.Error())
			}

			for _, n := range nodes {
				addr := n.GetAddress()

				msgSent := msgs[i]
				err = <-sender.Send(exampleMessage{value: msgSent}, addr)
				if err != nil {
					panic("failed to send: " + err.Error())
				}

				from, msgReceived, err := receiver.Recv(ctx)
				if err != nil {
					panic("failed to receive: " + err.Error())
				}

				require.True(t, from.Equal(addr))
				require.Equal(t, msgReceived, msgSent)
			}
		}(i, r)
	}
}

func generateMessages(nbNodes int) []string {
	var messages []string

	for i := 0; i < nbNodes; i++ {
		messages = append(messages, fmt.Sprintf("MSG%d", i))
	}

	return messages
}

func createPlayers(nodes []*Minogrpc) mino.Players {
	var addresses []mino.Address
	for _, n := range nodes {
		addresses = append(addresses, n.GetAddress())
	}

	players := mino.NewAddresses(addresses...)

	return players
}

func exchangeCertificates(nodes []*Minogrpc) {
	for _, n := range nodes {
		for _, m := range nodes {
			//if i != j {
			n.GetCertificateStore().Store(m.GetAddress(), m.GetCertificateChain())
			//}
		}
	}
}

func createRpcs(nodes []*Minogrpc) []mino.RPC {
	var rpcs []mino.RPC

	for _, n := range nodes {
		r := mino.MustCreateRPC(n, "test", exampleHandler{}, exampleFactory{})
		rpcs = append(rpcs, r)
	}

	return rpcs
}

func createNodes(nbNodes int, useTree bool) []*Minogrpc {
	var nodes []*Minogrpc
	for i := 0; i < nbNodes; i++ {
		var n *Minogrpc
		var err error

		if useTree {
			n, err = NewMinogrpc(ParseAddress("127.0.0.1", 0), nil, tree.NewRouter(NewAddressFactory()))
		} else {
			n, err = NewMinogrpc(ParseAddress("127.0.0.1", 0), nil, flat.NewRouter(NewAddressFactory()))
		}

		if err != nil {
			panic("overlay A failed: " + err.Error())
		} else {
			nodes = append(nodes, n)
		}
	}

	return nodes
}

func ExampleRPC_Call() {
	mA, err := NewMinogrpc(ParseAddress("127.0.0.1", 0), nil, tree.NewRouter(NewAddressFactory()))
	if err != nil {
		panic("overlay A failed: " + err.Error())
	}

	rpcA := mino.MustCreateRPC(mA, "test", exampleHandler{}, exampleFactory{})

	mB, err := NewMinogrpc(ParseAddress("127.0.0.1", 0), nil, tree.NewRouter(NewAddressFactory()))
	if err != nil {
		panic("overlay B failed: " + err.Error())
	}

	mino.MustCreateRPC(mB, "test", exampleHandler{}, exampleFactory{})

	mA.GetCertificateStore().Store(mB.GetAddress(), mB.GetCertificateChain())
	mB.GetCertificateStore().Store(mA.GetAddress(), mA.GetCertificateChain())

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
	mA, err := NewMinogrpc(ParseAddress("127.0.0.1", 0), nil, tree.NewRouter(NewAddressFactory()))
	if err != nil {
		panic("overlay A failed: " + err.Error())
	}

	rpcA := mino.MustCreateRPC(mA, "test", exampleHandler{}, exampleFactory{})

	mB, err := NewMinogrpc(ParseAddress("127.0.0.1", 0), nil, tree.NewRouter(NewAddressFactory()))
	if err != nil {
		panic("overlay B failed: " + err.Error())
	}

	mino.MustCreateRPC(mB, "test", exampleHandler{}, exampleFactory{})

	mA.GetCertificateStore().Store(mB.GetAddress(), mB.GetCertificateChain())
	mB.GetCertificateStore().Store(mA.GetAddress(), mA.GetCertificateChain())

	addrs := mino.NewAddresses(mA.GetAddress(), mB.GetAddress())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, receiver, err := rpcA.Stream(ctx, addrs)
	if err != nil {
		panic("stream failed: " + err.Error())
	}

	err = <-sender.Send(exampleMessage{value: "Hello World!"}, mB.GetAddress())
	if err != nil {
		panic("failed to send: " + err.Error())
	}

	from, msg, err := receiver.Recv(ctx)
	if err != nil {
		panic("failed to receive: " + err.Error())
	}

	if from.Equal(mB.GetAddress()) {
		fmt.Println("B", msg.(exampleMessage).value)
	}

	// Output: B Hello World!
}

func ExampleRPC_OpentracingDemo() {
	N := 20
	minos := make([]*Minogrpc, N)
	rpcs := make([]mino.RPC, N)

	for i := 0; i < N; i++ {
		m, err := NewMinogrpc(ParseAddress("127.0.0.1", 0), nil, tree.NewRouter(NewAddressFactory()))
		if err != nil {
			panic(fmt.Sprintf("overlay %d failed: %s", i, err.Error()))
		}
		minos[i] = m
		defer minos[i].GracefulStop()

		rpcs[i] = mino.MustCreateRPC(minos[i], "test", exampleHandler{}, exampleFactory{})
	}

	for i := 0; i < N; i++ {
		for j := 0; j < N; j++ {
			if i == j {
				continue
			}
			mA, mB := minos[i], minos[j]
			mA.GetCertificateStore().Store(mB.GetAddress(), mB.GetCertificateChain())
		}
	}

	addrs := make([]mino.Address, N)
	for i := 0; i < N; i++ {
		addrs[i] = minos[i].GetAddress()
	}
	minoAddrs := mino.NewAddresses(addrs...)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, tracing.ProtocolKey, "example-protocol")

	sender, recv, err := rpcs[0].Stream(ctx, minoAddrs)
	if err != nil {
		panic("stream failed: " + err.Error())
	}

	err = <-sender.Send(exampleMessage{value: "Hello World!"}, addrs[1:]...)
	if err != nil {
		panic("failed to send: " + err.Error())
	}

	for i := 0; i < N-1; i++ {
		from, msg, err := recv.Recv(ctx)
		if err != nil {
			panic("failed to receive: " + err.Error())
		}

		idx := -1
		for i := 1; i < N; i++ {
			if addrs[i].Equal(from) {
				idx = i
				break
			}
		}

		fmt.Printf("%d %s\n", idx, msg.(exampleMessage).value)
	}

	// Unordered output: 1 Hello World!
	// 2 Hello World!
	// 3 Hello World!
	// 4 Hello World!
	// 5 Hello World!
	// 6 Hello World!
	// 7 Hello World!
	// 8 Hello World!
	// 9 Hello World!
	// 10 Hello World!
	// 11 Hello World!
	// 12 Hello World!
	// 13 Hello World!
	// 14 Hello World!
	// 15 Hello World!
	// 16 Hello World!
	// 17 Hello World!
	// 18 Hello World!
	// 19 Hello World!

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
