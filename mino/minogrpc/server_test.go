package minogrpc

import (
	context "context"
	"strings"
	"testing"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// Use a single node to make a call that just sends back the same message.
func Test_SingleSimpleCall(t *testing.T) {
	identifier := "127.0.0.1:2000"

	addr := address{
		id: identifier,
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

	pba, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)

	msg := &Envelope{
		From:    addr.String(),
		To:      []string{addr.String()},
		Message: pba,
	}

	respChan, errChan := rpc.Call(msg, fakeNode{addr: addr})
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

			msg2, ok := resp.(*Envelope)
			require.True(t, ok)

			require.Equal(t, msg.From, msg2.From)
			require.Equal(t, len(msg.To), len(msg2.To))
			require.Equal(t, len(msg.To), 1)
			require.Equal(t, msg.To[0], msg2.To[0])

			msg := &empty.Empty{}
			err = ptypes.UnmarshalAny(msg2.Message, msg)
			require.NoError(t, err)

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

	addr := address{
		id: identifier,
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

	respChan, errChan := rpc.Call(&empty.Empty{}, fakeNode{addr: addr})
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

			_, ok = resp.(*empty.Empty)
			require.True(t, ok)

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
	addr1 := address{
		id: identifier1,
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
	addr2 := address{
		id: identifier2,
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
	addr3 := address{
		id: identifier3,
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
	respChan, errChan := rpc.Call(&empty.Empty{},
		fakeNode{addr: addr1}, fakeNode{addr: addr2}, fakeNode{addr: addr3})

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

			_, ok = resp.(*empty.Empty)
			require.True(t, ok)

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
	respChan, errChan = rpc.Call(&empty.Empty{},
		fakeNode{addr: addr1}, fakeNode{addr: addr2}, fakeNode{addr: addr3})

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

			_, ok = resp.(*empty.Empty)
			require.True(t, ok)

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

	addr := &address{
		id: identifier,
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

	m, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)

	msg := Envelope{
		From:    addr.String(),
		To:      []string{addr.String()},
		Message: m,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, receiver := rpc.Stream(ctx, simpleNode{addr: *addr})

	err = sender.Send(&msg, addr)
	require.NoError(t, err)

	_, msg2, err := receiver.Recv(context.Background())
	require.NoError(t, err)

	enveloppe, ok := msg2.(*Envelope)
	require.True(t, ok)
	empty2 := &empty.Empty{}
	err = ptypes.UnmarshalAny(enveloppe.Message, empty2)
	require.NoError(t, err)
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
	for {
		// ctx if I want a timeout
		addr, msg, err := in.Recv(context.Background())
		if err != nil {
			return xerrors.Errorf("failed to receive message in handler: %v", err)
		}

		err = out.Send(msg, addr)
		if err != nil {
			return xerrors.Errorf("failed to send message to the sender: %v", err)
		}
	}
}

// implements a handler interface that receives an address and adds a suffix to
// it
type testModifyHandler struct {
}

func (t testModifyHandler) Process(req proto.Message) (proto.Message, error) {
	msg, ok := req.(*empty.Empty)
	if !ok {
		return nil, xerrors.Errorf("failed to parse request")
	}

	return msg, nil
}

func (t testModifyHandler) Combine(req []proto.Message) ([]proto.Message, error) {
	return nil, nil
}

func (t testModifyHandler) Stream(out mino.Sender, in mino.Receiver) error {
	return nil
}

type fakeNode struct {
	addr address
}

func (n fakeNode) GetAddress() mino.Address {
	return n.addr
}
