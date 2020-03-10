package minogrpc

import (
	context "context"
	"fmt"
	"strings"
	"testing"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// Use a single node to make a call that just sends back the same message.
func Test_SingleSimpleCall(t *testing.T) {
	identifier := "127.0.0.1:2000"

	addr := &mino.Address{
		Id: identifier,
	}

	server, err := CreateServer(addr)
	require.NoError(t, err)
	err = server.StartServer()
	require.NoError(t, err)

	peer := Peer{
		Address:     server.listener.Addr().String(),
		Certificate: server.cert.Leaf,
	}
	server.neighbours[identifier] = peer

	handler := testSameHandler{}
	uri := "blabla"
	rpc := RPC{
		handler: handler,
		srv:     *server,
		uri:     uri,
	}

	server.handlers[uri] = handler

	m, err := ptypes.MarshalAny(addr)
	require.NoError(t, err)

	msg := mino.Envelope{
		From:    addr,
		To:      []*mino.Address{addr},
		Message: m,
	}

	respChan, errChan := rpc.Call(&msg, addr)
loop:
	for {
		select {
		case msgErr := <-errChan:
			t.Errorf("unexpected error: %v", msgErr)
			break loop
		case resp, ok := <-respChan:
			if !ok {
				break loop
			}

			msg2, ok := resp.(*mino.Envelope)
			require.True(t, ok)

			require.Equal(t, msg.From.Id, msg2.From.Id)
			require.Equal(t, len(msg.To), len(msg2.To))
			require.Equal(t, len(msg.To), 1)
			require.Equal(t, msg.To[0].Id, msg2.To[0].Id)

			addr2 := &mino.Address{}
			err = ptypes.UnmarshalAny(msg2.Message, addr2)
			require.NoError(t, err)
			require.Equal(t, addr.Id, addr2.Id)

		case <-time.After(2 * time.Second):
			break loop
		}
	}

	server.grpcSrv.GracefulStop()
	err = server.httpSrv.Shutdown(context.Background())
	require.NoError(t, err)

}

// Using a single node to make a call that sends back a modified message.
func Test_SingleModifyCall(t *testing.T) {
	identifier := "127.0.0.1:2000"

	addr := &mino.Address{
		Id: identifier,
	}

	server, err := CreateServer(addr)
	require.NoError(t, err)
	err = server.StartServer()
	require.NoError(t, err)

	peer := Peer{
		Address:     server.listener.Addr().String(),
		Certificate: server.cert.Leaf,
	}
	server.neighbours[identifier] = peer

	handler := testModifyHandler{}
	uri := "blabla"
	rpc := RPC{
		handler: handler,
		srv:     *server,
		uri:     uri,
	}

	server.handlers[uri] = handler

	respChan, errChan := rpc.Call(addr, addr)
loop:
	for {
		select {
		case msgErr := <-errChan:
			t.Errorf("unexpected error: %v", msgErr)
			break loop
		case resp, ok := <-respChan:
			if !ok {
				break loop
			}

			addr2, ok := resp.(*mino.Address)
			require.NoError(t, err)
			require.Equal(t, addr.Id+"suffix", addr2.Id)

		case <-time.After(2 * time.Second):
			break loop
		}
	}

	server.grpcSrv.GracefulStop()
	err = server.httpSrv.Shutdown(context.Background())
	require.NoError(t, err)
}

// Using 3 nodes to make a call that sends back a modified message.
func Test_MultipleModifyCall(t *testing.T) {
	// Server 1
	identifier1 := "127.0.0.1:2001"
	addr1 := &mino.Address{
		Id: identifier1,
	}
	server1, err := CreateServer(addr1)
	require.NoError(t, err)
	err = server1.StartServer()
	require.NoError(t, err)
	peer1 := Peer{
		Address:     server1.listener.Addr().String(),
		Certificate: server1.cert.Leaf,
	}

	// Server 2
	identifier2 := "127.0.0.1:2002"
	addr2 := &mino.Address{
		Id: identifier2,
	}
	server2, err := CreateServer(addr2)
	err = server2.StartServer()
	require.NoError(t, err)
	require.NoError(t, err)
	peer2 := Peer{
		Address:     server2.listener.Addr().String(),
		Certificate: server2.cert.Leaf,
	}

	// Server 3
	identifier3 := "127.0.0.1:2003"
	addr3 := &mino.Address{
		Id: identifier3,
	}
	server3, err := CreateServer(addr3)
	err = server3.StartServer()
	require.NoError(t, err)
	require.NoError(t, err)
	peer3 := Peer{
		Address:     server3.listener.Addr().String(),
		Certificate: server3.cert.Leaf,
	}

	// Update the list of peers for server1
	server1.neighbours[identifier1] = peer1
	server1.neighbours[identifier2] = peer2
	server1.neighbours[identifier3] = peer3

	// Set the handlers on each server
	handler := testModifyHandler{}
	uri := "blabla"
	rpc := RPC{
		handler: handler,
		srv:     *server1,
		uri:     uri,
	}

	server1.handlers[uri] = handler
	server2.handlers[uri] = handler
	server3.handlers[uri] = handler

	// Call the rpc on server1
	respChan, errChan := rpc.Call(addr1, addr1, addr2, addr3)

	// To track the number of message we got back. Should be 3
	numRequests := 0
loop:
	for {
		select {
		case msgErr := <-errChan:
			t.Errorf("unexpected error: %v", msgErr)
			break loop
		case resp, ok := <-respChan:
			if !ok {
				break loop
			}

			respAddr, ok := resp.(*mino.Address)
			require.NoError(t, err)
			require.Equal(t, addr1.Id+"suffix", respAddr.Id)

			numRequests++

		case <-time.After(2 * time.Second):
			break loop
		}
	}

	require.Equal(t, 3, numRequests)

	//
	// Doing the same but closing server3
	//
	server3.grpcSrv.GracefulStop()
	err = server3.httpSrv.Shutdown(context.Background())
	require.NoError(t, err)

	// Call the rpc on server1
	respChan, errChan = rpc.Call(addr1, addr1, addr2, addr3)

	// To track the number of message we got back. Should be 2
	numRequests = 0
	// Track the number of expected errors. Should be 1
	numExpectedErrors := 0
loop2:
	for {
		select {
		case msgErr := <-errChan:
			if strings.HasPrefix(msgErr.Error(), "failed to call client: ") {
				numExpectedErrors++
			} else {
				t.Errorf("unexpected error: %v", msgErr)
				break loop2
			}
		case resp, ok := <-respChan:
			if !ok {
				break loop2
			}

			respAddr, ok := resp.(*mino.Address)
			require.NoError(t, err)
			require.Equal(t, addr1.Id+"suffix", respAddr.Id)

			numRequests++

		case <-time.After(2 * time.Second):
			break loop2
		}
	}

	require.Equal(t, 2, numRequests)
	require.Equal(t, 1, numExpectedErrors)

	// Closing servers 1 and 2

	server1.grpcSrv.GracefulStop()
	err = server1.httpSrv.Shutdown(context.Background())
	require.NoError(t, err)

	server2.grpcSrv.GracefulStop()
	err = server2.httpSrv.Shutdown(context.Background())
	require.NoError(t, err)

}

// Use a single node to make a stream that just sends back the same message.
func Test_SingleSimpleStream(t *testing.T) {
	identifier := "127.0.0.1:2000"

	addr := &mino.Address{
		Id: identifier,
	}

	server, err := CreateServer(addr)
	require.NoError(t, err)
	err = server.StartServer()
	require.NoError(t, err)

	peer := Peer{
		Address:     server.listener.Addr().String(),
		Certificate: server.cert.Leaf,
	}
	server.neighbours[identifier] = peer

	handler := testSameHandler{}
	uri := "blabla"
	rpc := RPC{
		handler: handler,
		srv:     *server,
		uri:     uri,
	}

	server.handlers[uri] = handler

	m, err := ptypes.MarshalAny(addr)
	require.NoError(t, err)

	msg := mino.Envelope{
		From:    addr,
		To:      []*mino.Address{addr},
		Message: m,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("before rpc.Stream")
	sender, receiver := rpc.Stream(ctx, addr)
	fmt.Println("after rpc.Stream")

	fmt.Println("\nbefore the send")
	err = sender.Send(&msg, addr)
	require.NoError(t, err)
	fmt.Println("after the send")

	fmt.Println("\nbefore revc")
	_, msg2, err := receiver.Recv(context.Background())
	fmt.Println("after recv")
	require.NoError(t, err)

	enveloppe, ok := msg2.(*mino.Envelope)
	require.True(t, ok)
	addr2 := &mino.Address{}
	err = ptypes.UnmarshalAny(enveloppe.Message, addr2)
	require.NoError(t, err)
	require.Equal(t, addr.Id, addr2.Id)
}

// -------
// Utility functions

// implements a handler interface that just returns the input
type testSameHandler struct {
}

func (t testSameHandler) Process(req proto.Message) (proto.Message, error) {
	return req, nil
}

func (t testSameHandler) Combine(req []proto.Message) ([]proto.Message, error) {
	return nil, nil
}

// Stream is a dummy handler that forwards input messages to the sender
func (t testSameHandler) Stream(out mino.Sender, in mino.Receiver) error {
	fmt.Println("inside the Stream handler")
	count := 0
	for {
		fmt.Println("count = ", count)
		if count >= 1 {
			return nil
		}
		// ctx if I want a timeout
		fmt.Println("handler calling in.Recv")
		addr, msg, err := in.Recv(context.Background())
		if err != nil {
			return xerrors.Errorf("failed to receive message in handler: %v", err)
		}

		fmt.Printf("from the handler, got this message: %v\n", msg)

		err = out.Send(msg, addr)
		if err != nil {
			return xerrors.Errorf("failed to send message to the sender: %v", err)
		}

		count++
	}
}

// implements a handler interface that receives an address and adds a suffix to
// it
type testModifyHandler struct {
}

func (t testModifyHandler) Process(req proto.Message) (proto.Message, error) {
	addr, ok := req.(*mino.Address)
	if !ok {
		return nil, xerrors.Errorf("failed to parse request")
	}

	addr.Id = addr.Id + "suffix"

	return addr, nil
}

func (t testModifyHandler) Combine(req []proto.Message) ([]proto.Message, error) {
	return nil, nil
}

func (t testModifyHandler) Stream(out mino.Sender, in mino.Receiver) error {
	return nil
}
