package minogrpc

import (
	context "context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

func Test_NewServer(t *testing.T) {
	// Using an empty address should yield an error
	addr := address{}
	_, err := NewServer(addr, TreeRoutingFactory)
	require.EqualError(t, err, "addr.String() should not give an empty string")
}

func TestServer_Serve(t *testing.T) {
	server := Server{
		addr: address{id: "blabla"},
	}
	err := server.Serve()
	// We need to provide a port
	require.EqualError(t, err, "failed to listen: listen tcp4: address blabla: missing port in address")
	// The host should be resolvable
	server.addr = address{id: "blabla:2000"}
	err = server.Serve()
	require.True(t, strings.HasPrefix(err.Error(), "failed to listen: listen tcp4: lookup blabla"))
}

func TestGetConnection(t *testing.T) {
	addr := &address{
		id: "127.0.0.1:2000",
	}

	server, err := NewServer(addr, TreeRoutingFactory)
	require.NoError(t, err)
	server.StartServer()

	// An empty address should yield an error
	_, err = getConnection("", Peer{}, *server.cert)
	require.EqualError(t, err, "empty address is not allowed")

	server.grpcSrv.GracefulStop()
}

// Use a single node to make a call that just sends back the same message.
func TestRPC_SingleSimple_Call(t *testing.T) {
	identifier := "127.0.0.1:2000"

	addr := address{
		id: identifier,
	}

	server, err := NewServer(addr, TreeRoutingFactory)
	require.NoError(t, err)
	server.StartServer()

	peer := Peer{
		Address:     server.listener.Addr().String(),
		Certificate: server.cert.Leaf,
	}
	server.neighbours[identifier] = peer

	handler := testSameHandler{time.Millisecond * 200}
	uri := "blabla"
	rpc := RPC{
		encoder: encoding.NewProtoEncoder(),
		handler: handler,
		srv:     server,
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

	ctx := context.Background()

	respChan, errChan := rpc.Call(ctx, msg, fakeMembership{addrs: []address{addr}})
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
	require.NoError(t, err)
}

func TestRPC_ErrorsSimple_Call(t *testing.T) {
	identifier := "127.0.0.1:2000"

	addr := address{
		id: identifier,
	}

	server, err := NewServer(addr, TreeRoutingFactory)
	require.NoError(t, err)
	server.StartServer()

	handler := testSameHandler{time.Millisecond * 200}
	uri := "blabla"
	rpc := RPC{
		handler: handler,
		srv:     server,
		uri:     uri,
	}

	ctx := context.Background()

	// Using a wrong request message (nil) should yield an error while decoding
	respChan, errChan := rpc.Call(ctx, nil, &fakePlayers{players: []address{addr}})
loop:
	for {
		select {
		case msgErr := <-errChan:
			require.EqualError(t, msgErr, "failed to marshal msg to any: proto: Marshal called with nil")
			break loop
		case resp, ok := <-respChan:
			if !ok {
				break loop
			}
			t.Errorf("unexpected message received: %v", resp)
		case <-time.After(2 * time.Second):
			break loop
		}
	}

	// It should fail to get the client connection because the given addresse in
	// fakeNode is not in the roster
	pba, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)

	msg := &Envelope{
		From:    addr.String(),
		To:      []string{addr.String()},
		Message: pba,
	}
	respChan, errChan = rpc.Call(ctx, msg, &fakePlayers{players: []address{addr}})
loop2:
	for {
		select {
		case msgErr := <-errChan:
			require.EqualError(t, msgErr, "addr '127.0.0.1:2000' not is our list of neighbours")
			break loop2
		case resp, ok := <-respChan:
			if !ok {
				break loop2
			}
			t.Errorf("unexpected message received: %v", resp)
		case <-time.After(2 * time.Second):
			break loop2
		}
	}

	server.grpcSrv.GracefulStop()
}

// Using a single node to make a call that sends back a modified message.
func TestRPC_SingleModify_Call(t *testing.T) {
	identifier := "127.0.0.1:2000"

	addr := address{
		id: identifier,
	}

	server, err := NewServer(addr, TreeRoutingFactory)
	require.NoError(t, err)
	server.StartServer()

	peer := Peer{
		Address:     server.listener.Addr().String(),
		Certificate: server.cert.Leaf,
	}
	server.neighbours[identifier] = peer

	handler := testModifyHandler{}
	uri := "blabla"
	rpc := RPC{
		encoder: encoding.NewProtoEncoder(),
		handler: handler,
		srv:     server,
		uri:     uri,
	}

	server.handlers[uri] = handler

	ctx := context.Background()

	respChan, errChan := rpc.Call(ctx, &empty.Empty{}, fakeMembership{addrs: []address{addr}})
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
}

// Using 3 nodes to make a call that sends back a modified message.
func TestRPC_MultipleModify_Call(t *testing.T) {
	// Server 1
	identifier1 := "127.0.0.1:2001"
	addr1 := address{
		id: identifier1,
	}
	server1, err := NewServer(addr1, TreeRoutingFactory)
	require.NoError(t, err)
	server1.StartServer()
	peer1 := Peer{
		Address:     server1.listener.Addr().String(),
		Certificate: server1.cert.Leaf,
	}

	// Server 2
	identifier2 := "127.0.0.1:2002"
	addr2 := address{
		id: identifier2,
	}
	server2, err := NewServer(addr2, TreeRoutingFactory)
	require.NoError(t, err)
	server2.StartServer()
	peer2 := Peer{
		Address:     server2.listener.Addr().String(),
		Certificate: server2.cert.Leaf,
	}

	// Server 3
	identifier3 := "127.0.0.1:2003"
	addr3 := address{
		id: identifier3,
	}
	server3, err := NewServer(addr3, TreeRoutingFactory)
	require.NoError(t, err)
	server3.StartServer()
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
		encoder: encoding.NewProtoEncoder(),
		handler: handler,
		srv:     server1,
		uri:     uri,
	}

	server1.handlers[uri] = handler
	server2.handlers[uri] = handler
	server3.handlers[uri] = handler

	memship := fakeMembership{addrs: []address{addr1, addr2, addr3}}

	ctx := context.Background()

	// Call the rpc on server1
	respChan, errChan := rpc.Call(ctx, &empty.Empty{}, memship)

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

	// Call the rpc on server1
	respChan, errChan = rpc.Call(ctx, &empty.Empty{}, memship)

	// To track the number of message we got back. Should be 2
	numRequests = 0
	// Track the number of expected errors. Should be 1
	numExpectedErrors := 0
loop2:
	for {
		select {
		case msgErr := <-errChan:
			if strings.HasPrefix(msgErr.Error(), "failed to call client '127.0.0.1:2003': ") {
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
	server2.grpcSrv.GracefulStop()

}

// Use a 3 nodes to make a stream that just sends back the same message.
func TestRPC_SingleSimple_Stream(t *testing.T) {
	identifier := "127.0.0.1:2000"

	addr := &address{
		id: identifier,
	}

	server, err := NewServer(addr, TreeRoutingFactory)
	require.NoError(t, err)
	server.StartServer()

	peer := Peer{
		Address:     server.listener.Addr().String(),
		Certificate: server.cert.Leaf,
	}
	server.neighbours[identifier] = peer

	handler := testSameHandler{time.Millisecond * 200}
	uri := "blabla"
	rpc := RPC{
		encoder:        encoding.NewProtoEncoder(),
		handler:        handler,
		srv:            server,
		uri:            uri,
		routingFactory: TreeRoutingFactory,
	}

	server.handlers[uri] = handler

	m := &empty.Empty{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, receiver := rpc.Stream(ctx, &fakePlayers{players: []address{*addr}})

	errs := sender.Send(m, addr)
	err, more := <-errs
	if more {
		t.Error("unexpected error from send: ", err)
	}

	_, msg2, err := receiver.Recv(context.Background())
	require.NoError(t, err)

	_, ok := msg2.(*empty.Empty)
	require.True(t, ok)

	server.grpcSrv.GracefulStop()

}

// Use a single node to make a stream that just sends back the same message.
func TestRPC_ErrorsSimple_Stream(t *testing.T) {
	identifier := "127.0.0.1:2000"

	addr := &address{
		id: identifier,
	}

	server, err := NewServer(addr, TreeRoutingFactory)
	require.NoError(t, err)
	server.StartServer()

	peer := Peer{
		Address:     server.listener.Addr().String(),
		Certificate: server.cert.Leaf,
	}
	server.neighbours[identifier] = peer

	handler := testSameHandler{time.Millisecond * 200}
	uri := "blabla"
	rpc := RPC{
		encoder:        encoding.NewProtoEncoder(),
		handler:        handler,
		srv:            server,
		uri:            uri,
		routingFactory: TreeRoutingFactory,
	}

	server.handlers[uri] = handler

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Using an empty address should yield an error
	_, receiver := rpc.Stream(ctx, &fakePlayers{players: []address{{}}})

	_, _, err = receiver.Recv(context.Background())
	require.EqualError(t, err, "got an error from the error chan: addr '' not is our list of neighbours")

	server.grpcSrv.GracefulStop()

}

// Use multiple nodes to use a stream that just sends back the same message.
func TestRPC_MultipleSimple_Stream(t *testing.T) {
	identifier1 := "127.0.0.1:2001"
	addr1 := &address{
		id: identifier1,
	}
	server1, err := NewServer(addr1, TreeRoutingFactory)
	require.NoError(t, err)
	server1.StartServer()
	peer1 := Peer{
		Address:     server1.listener.Addr().String(),
		Certificate: server1.cert.Leaf,
	}

	identifier2 := "127.0.0.1:2002"
	addr2 := &address{
		id: identifier2,
	}
	server2, err := NewServer(addr2, TreeRoutingFactory)
	require.NoError(t, err)
	server2.StartServer()
	peer2 := Peer{
		Address:     server2.listener.Addr().String(),
		Certificate: server2.cert.Leaf,
	}

	identifier3 := "127.0.0.1:2003"
	addr3 := &address{
		id: identifier3,
	}
	server3, err := NewServer(addr3, TreeRoutingFactory)
	require.NoError(t, err)
	require.NoError(t, err)
	server3.StartServer()
	peer3 := Peer{
		Address:     server3.listener.Addr().String(),
		Certificate: server3.cert.Leaf,
	}

	// Computed routing:
	//
	// Node[orchestrator_addr-index[-1]-lastIndex[2]](
	// 	Node[127.0.0.1:2001-index[0]-lastIndex[0]](
	// 	)
	// 	Node[127.0.0.1:2002-index[1]-lastIndex[2]](
	// 		Node[127.0.0.1:2003-index[2]-lastIndex[2]](
	// 		)
	// 	)
	// )

	server1.neighbours[identifier1] = peer1
	server1.neighbours[identifier2] = peer2

	server2.neighbours[identifier3] = peer3

	handler := testSameHandler{time.Millisecond * 900}
	uri := "blabla"
	rpc := RPC{
		encoder:        encoding.NewProtoEncoder(),
		handler:        handler,
		srv:            server1,
		uri:            uri,
		routingFactory: TreeRoutingFactory,
	}

	// the handler must be registered on each server. Fron the client side, that
	// means the "registerNamespace" and "makeRPC" must be called on each
	// server.
	server1.handlers[uri] = handler
	server2.handlers[uri] = handler
	server3.handlers[uri] = handler

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, rcvr := rpc.Stream(ctx, &fakePlayers{players: []address{*addr1, *addr2, *addr3}})

	localRcvr, ok := rcvr.(receiver)
	require.True(t, ok)

	select {
	case err := <-localRcvr.errs:
		t.Errorf("unexpected error in rcvr: %v", err)
	case <-time.After(time.Millisecond * 200):
	}

	// sending to one server and checking if we got an empty back
	errs := sender.Send(&empty.Empty{}, addr1)
	err, more := <-errs
	if more {
		t.Error("unexpected error from send: ", err)
	}

	_, msg2, err := rcvr.Recv(context.Background())
	require.NoError(t, err)

	_, ok = msg2.(*empty.Empty)
	require.True(t, ok)

	// sending to three servers
	errs = sender.Send(&empty.Empty{}, addr1, addr2, addr3)
	err, more = <-errs
	if more {
		t.Error("unexpected error from send: ", err)
	}

	// we should get three responses
	for i := 0; i < 3; i++ {
		_, msg2, err := rcvr.Recv(context.Background())
		require.NoError(t, err)
		_, ok := msg2.(*empty.Empty)
		require.True(t, ok)
	}

	server1.grpcSrv.GracefulStop()
	server2.grpcSrv.GracefulStop()
	server3.grpcSrv.GracefulStop()
	require.NoError(t, err)
}

// Use multiple nodes to use a stream that aggregates the dummyMessages
func TestRPC_MultipleChange_Stream(t *testing.T) {
	identifier1 := "127.0.0.1:2001"
	addr1 := &address{
		id: identifier1,
	}
	server1, err := NewServer(addr1, TreeRoutingFactory)
	require.NoError(t, err)
	server1.StartServer()
	peer1 := Peer{
		Address:     server1.listener.Addr().String(),
		Certificate: server1.cert.Leaf,
	}

	identifier2 := "127.0.0.1:2002"
	addr2 := &address{
		id: identifier2,
	}
	server2, err := NewServer(addr2, TreeRoutingFactory)
	require.NoError(t, err)
	server2.StartServer()
	peer2 := Peer{
		Address:     server2.listener.Addr().String(),
		Certificate: server2.cert.Leaf,
	}

	identifier3 := "127.0.0.1:2003"
	addr3 := &address{
		id: identifier3,
	}
	server3, err := NewServer(addr3, TreeRoutingFactory)
	require.NoError(t, err)
	server3.StartServer()
	peer3 := Peer{
		Address:     server3.listener.Addr().String(),
		Certificate: server3.cert.Leaf,
	}

	identifier4 := "127.0.0.1:2004"
	addr4 := &address{
		id: identifier4,
	}
	server4, err := NewServer(addr4, TreeRoutingFactory)
	require.NoError(t, err)
	server4.traffic.log = true
	server4.StartServer()
	peer4 := Peer{
		Address:     server4.listener.Addr().String(),
		Certificate: server4.cert.Leaf,
	}

	// Computed routing:
	//
	// Node[orchestrator_addr-index[-1]-lastIndex[3]](
	// 	Node[127.0.0.1:2002-index[0]-lastIndex[1]](
	// 		Node[127.0.0.1:2003-index[1]-lastIndex[1]](
	// 		)
	// 	)
	// 	Node[127.0.0.1:2004-index[2]-lastIndex[3]](
	// 		Node[127.0.0.1:2001-index[3]-lastIndex[3]](
	// 		)
	// 	)
	// )
	server1.neighbours[identifier1] = peer1
	server1.neighbours[identifier2] = peer2
	server1.neighbours[identifier4] = peer4

	server2.neighbours[identifier2] = peer2
	server2.neighbours[identifier3] = peer3

	server3.neighbours[identifier3] = peer3

	server4.neighbours[identifier1] = peer1

	uri := "blabla"
	rpc := RPC{
		encoder:        encoding.NewProtoEncoder(),
		srv:            server1,
		uri:            uri,
		routingFactory: TreeRoutingFactory,
	}

	// the handler must be registered on each server. Fron the client side, that
	// means the "registerNamespace" and "makeRPC" must be called on each
	// server.
	server1.handlers[uri] = testModifyHandler{identifier1}
	server2.handlers[uri] = testModifyHandler{identifier2}
	server3.handlers[uri] = testModifyHandler{identifier3}
	server4.handlers[uri] = testModifyHandler{identifier4}

	m := &wrappers.StringValue{Value: "dummy_value"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, rcvr := rpc.Stream(ctx, &fakePlayers{players: []address{*addr1, *addr2, *addr3, *addr4}})
	localRcvr, ok := rcvr.(receiver)
	require.True(t, ok)

	select {
	case err := <-localRcvr.errs:
		t.Errorf("unexpected error in rcvr: %v", err)
	case <-time.After(time.Millisecond * 200):
	}

	// sending two messages, we should get one from each server
	errs := sender.Send(m, addr1, addr2, addr3, addr4)
	err, more := <-errs
	if more {
		t.Error("unexpected error from send: ", err)
	}

	errs = sender.Send(m, addr1, addr2, addr3, addr4)
	err, more = <-errs
	if more {
		t.Error("unexpected error from send: ", err)
	}

	msgs := make([]string, 4)

	for i := range msgs {
		_, msg2, err := rcvr.Recv(context.Background())
		require.NoError(t, err)

		dummyMsg2, ok := msg2.(*wrappers.StringValue)
		require.True(t, ok)
		msgs[i] = dummyMsg2.Value
	}

	sort.Strings(msgs)
	require.Equal(t, "dummy_valuedummy_value_"+identifier1, msgs[0])
	require.Equal(t, "dummy_valuedummy_value_"+identifier2, msgs[1])
	require.Equal(t, "dummy_valuedummy_value_"+identifier3, msgs[2])
	require.Equal(t, "dummy_valuedummy_value_"+identifier4, msgs[3])

	server1.grpcSrv.GracefulStop()
	server2.grpcSrv.GracefulStop()
	server3.grpcSrv.GracefulStop()
	server4.grpcSrv.GracefulStop()
	require.NoError(t, err)
}

// Sends a message to 4 participants, but the server where the rpc is sent from
// only knows itself and a second one.
func TestRPC_MultipleRingMesh_Stream(t *testing.T) {
	oldTH := treeHeight
	treeHeight = 4
	defer func() {
		treeHeight = oldTH
	}()

	identifier1, addr1, server1, peer1 := createServer(t, "127.0.0.1:2001")
	identifier2, addr2, server2, peer2 := createServer(t, "127.0.0.1:2002")
	identifier3, addr3, server3, peer3 := createServer(t, "127.0.0.1:2003")
	identifier4, addr4, server4, peer4 := createServer(t, "127.0.0.1:2004")
	identifier5, addr5, server5, peer5 := createServer(t, "127.0.0.1:2005")
	identifier6, addr6, server6, peer6 := createServer(t, "127.0.0.1:2006")
	identifier7, addr7, server7, peer7 := createServer(t, "127.0.0.1:2007")
	identifier8, addr8, server8, peer8 := createServer(t, "127.0.0.1:2008")
	identifier9, addr9, server9, peer9 := createServer(t, "127.0.0.1:2009")

	// Computed routing:
	//
	// Node[orchestrator_addr-index[-1]-lastIndex[8]](
	// 	Node[127.0.0.1:2006-index[0]-lastIndex[3]](
	// 		Node[127.0.0.1:2007-index[1]-lastIndex[1]](
	// 		)
	// 		Node[127.0.0.1:2001-index[2]-lastIndex[3]](
	// 			Node[127.0.0.1:2002-index[3]-lastIndex[3]](
	// 			)
	// 		)
	// 	)
	// 	Node[127.0.0.1:2003-index[4]-lastIndex[8]](
	// 		Node[127.0.0.1:2005-index[5]-lastIndex[6]](
	// 			Node[127.0.0.1:2004-index[6]-lastIndex[6]](
	// 			)
	// 		)
	// 		Node[127.0.0.1:2008-index[7]-lastIndex[8]](
	// 			Node[127.0.0.1:2009-index[8]-lastIndex[8]](
	// 			)
	// 		)
	// 	)
	// )
	server1.neighbours[identifier2] = peer2
	server1.neighbours[identifier6] = peer6 // because server1 is the
	server1.neighbours[identifier3] = peer3 // orchestrator

	server2.neighbours[identifier2] = peer2
	server2.neighbours[identifier3] = peer3

	server3.neighbours[identifier5] = peer5
	server3.neighbours[identifier8] = peer8

	server5.neighbours[identifier4] = peer4

	server6.neighbours[identifier7] = peer7
	server6.neighbours[identifier1] = peer1

	server8.neighbours[identifier9] = peer9

	uri := "blabla"
	handler1 := testRingHandler{addrID: addr1, neighbor: addr2}
	rpc := RPC{
		encoder:        encoding.NewProtoEncoder(),
		handler:        handler1,
		srv:            server1,
		uri:            uri,
		routingFactory: TreeRoutingFactory,
	}

	// the handler must be registered on each server. Fron the client side, that
	// means the "registerNamespace" and "makeRPC" must be called on each
	// server.
	server1.handlers[uri] = handler1
	server2.handlers[uri] = testRingHandler{addrID: addr2, neighbor: addr3}
	server3.handlers[uri] = testRingHandler{addrID: addr3, neighbor: addr4}
	server4.handlers[uri] = testRingHandler{addrID: addr4, neighbor: addr5}
	server5.handlers[uri] = testRingHandler{addrID: addr5, neighbor: addr6}
	server6.handlers[uri] = testRingHandler{addrID: addr6, neighbor: addr7}
	server7.handlers[uri] = testRingHandler{addrID: addr7, neighbor: addr8}
	server8.handlers[uri] = testRingHandler{addrID: addr8, neighbor: addr9}
	server9.handlers[uri] = testRingHandler{addrID: addr9, neighbor: nil}

	dummyMsg := &wrappers.StringValue{Value: "dummy_value"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, rcvr := rpc.Stream(ctx, &fakePlayers{players: []address{
		*addr1, *addr2, *addr3, *addr4, *addr5, *addr6, *addr7, *addr8, *addr9},
	})
	localRcvr, ok := rcvr.(receiver)
	require.True(t, ok)

	time.Sleep(time.Second)

	select {
	case err := <-localRcvr.errs:
		t.Errorf("unexpected error in rcvr: %v", err)
	case <-time.After(time.Millisecond * 200):
	}

	// sending message to server1, which should send to its neighbor, etc...
	errs := sender.Send(dummyMsg, addr1)
	err, more := <-errs
	if more {
		t.Error("unexpected error from send: ", err)
	}

	_, msg2, err := rcvr.Recv(context.Background())
	require.NoError(t, err)

	dummyMsg2, ok := msg2.(*wrappers.StringValue)
	require.True(t, ok)

	require.Equal(t, dummyMsg2.Value, "dummy_value_127.0.0.1:2001_127.0.0.1:2002_127.0.0.1:2003_127.0.0.1:2004_127.0.0.1:2005_127.0.0.1:2006_127.0.0.1:2007_127.0.0.1:2008_127.0.0.1:2009")

	// out := os.Stdout
	// out.WriteString("\nserver1:\n")
	// server1.traffic.Display(out)
	// out.WriteString("\nserver2:\n")
	// server2.traffic.Display(out)
	// out.WriteString("\nserver3:\n")
	// server3.traffic.Display(out)
	// out.WriteString("\nserver4:\n")
	// server4.traffic.Display(out)

	GenerateGraphviz(os.Stdout, server1.traffic, server2.traffic,
		server3.traffic, server4.traffic, server5.traffic, server5.traffic,
		server6.traffic, server7.traffic, server8.traffic, server9.traffic)

	server1.grpcSrv.GracefulStop()
	server2.grpcSrv.GracefulStop()
	server3.grpcSrv.GracefulStop()
	server4.grpcSrv.GracefulStop()
}

func TestSender_Send(t *testing.T) {
	routing, err := TreeRoutingFactory.FromAddrs([]mino.Address{address{"0"}},
		map[string]interface{}{
			TreeRoutingOpts.Addr:       address{"0"},
			TreeRoutingOpts.TreeHeight: 1,
		})
	require.NoError(t, err)

	sender := sender{
		encoder:      encoding.NewProtoEncoder(),
		participants: make(map[string]overlayStream),
		routing:      routing,
	}

	// sending to an empty list should not yield an error
	errs := sender.Send(nil)
	err, more := <-errs
	if more {
		t.Error("unexpected error from send: ", err)
	}

	// giving an empty address should add an error since it won't be found in
	// the list of participants
	addr := address{}
	errs = sender.Send(&empty.Empty{}, addr)
	err, more = <-errs
	if !more {
		t.Error("there should be an error")
	}
	require.EqualError(t, err, "sender '' failed to send to client '': failed to send a message to my child '', participant not found in 'map[]'")

	// now I add the participant to the list, an error should be given since the
	// message is nil
	addr = address{id: "fake"}
	sender.encoder = badMarshalAnyEncoder{}
	sender.participants[addr.String()] = nil
	errs = sender.Send(nil, addr)
	err, more = <-errs
	if !more {
		t.Error("there should be an error")
	}
	require.EqualError(t, err,
		"sender '' failed to send to client 'fake': couldn't marshal message: oops")
}

func TestReceiver_Recv(t *testing.T) {
	receiver := receiver{
		encoder: encoding.NewProtoEncoder(),
		errs:    make(chan error, 1),
		in:      make(chan *OverlayMsg, 1),
	}

	// If there is a wrong message (nil), then it should output an error
	receiver.in <- nil
	_, _, err := receiver.Recv(context.Background())
	require.EqualError(t, err, "message is nil")

	// now with a failing unmarshal of the envelope
	msg := &OverlayMsg{
		Message: nil,
	}
	receiver.encoder = badUnmarshalAnyEncoder{}
	receiver.in <- msg
	_, _, err = receiver.Recv(context.Background())
	require.EqualError(t, err, "couldn't unmarshal envelope: oops")

	// now with a failing unmarshal of the message
	msg.Message, err = ptypes.MarshalAny(&Envelope{})
	require.NoError(t, err)
	receiver.encoder = badUnmarshalDynEncoder{}
	receiver.in <- msg
	_, _, err = receiver.Recv(context.Background())
	require.EqualError(t, err, "couldn't unmarshal message: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type badMarshalAnyEncoder struct {
	encoding.ProtoEncoder
}

func (e badMarshalAnyEncoder) MarshalAny(proto.Message) (*any.Any, error) {
	return nil, xerrors.New("oops")
}

type badUnmarshalAnyEncoder struct {
	encoding.ProtoEncoder
}

func (e badUnmarshalAnyEncoder) UnmarshalAny(*any.Any, proto.Message) error {
	return xerrors.New("oops")
}

type badUnmarshalDynEncoder struct {
	encoding.ProtoEncoder
}

func (e badUnmarshalDynEncoder) UnmarshalDynamicAny(*any.Any) (proto.Message, error) {
	return nil, xerrors.New("oops")
}

// Handler:
// implements a handler interface that just returns the input
type testSameHandler struct {
	timeout time.Duration
}

func (t testSameHandler) Process(req mino.Request) (proto.Message, error) {
	return req.Message, nil
}

func (t testSameHandler) Combine(req []proto.Message) ([]proto.Message, error) {
	return nil, nil
}

// Stream is a dummy handler that forwards input messages to the sender
func (t testSameHandler) Stream(out mino.Sender, in mino.Receiver) error {
	for {
		ctx, cancelFunc := context.WithTimeout(context.Background(), t.timeout)
		defer cancelFunc()
		addr, msg, err := in.Recv(ctx)
		if err == context.DeadlineExceeded {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("failed to receive message in handler: %v", err)
		}
		errs := out.Send(msg, addr)
		err, more := <-errs
		if more {
			return xerrors.Errorf("failed to send message to the sender: %v", err)
		}
	}
}

// Handler:
// implements a handler interface that receives an address and adds a suffix to
// it (for the call) or aggregate all the address (for the stream). The stream
// expects 3 calls before returning the aggregate addresses.
type testModifyHandler struct {
	addrID string
}

func (t testModifyHandler) Process(req mino.Request) (proto.Message, error) {
	msg, ok := req.Message.(*empty.Empty)
	if !ok {
		return nil, xerrors.Errorf("failed to parse request")
	}

	return msg, nil
}

func (t testModifyHandler) Combine(req []proto.Message) ([]proto.Message, error) {
	return nil, nil
}

// This function reads two messages and outputs an aggregation
func (t testModifyHandler) Stream(out mino.Sender, in mino.Receiver) error {
	var dummyMsg string
	var addr mino.Address
	for i := 0; i < 2; i++ {
		var msg proto.Message
		var err error
		// ctx if I want a timeout
		addr, msg, err = in.Recv(context.Background())
		if err != nil {
			return xerrors.Errorf("failed to receive message in handler: %v", err)
		}

		dummy, ok := msg.(*wrappers.StringValue)
		if !ok {
			return xerrors.Errorf("failed to cast message to StringValue: %T", msg)
		}

		dummyMsg += dummy.Value
	}
	stringMsg := dummyMsg + "_" + t.addrID
	dummyReturn := &wrappers.StringValue{Value: stringMsg}

	errs := out.Send(dummyReturn, addr)
	err, more := <-errs
	if more {
		return xerrors.Errorf("failed to send message to the sender: %v", err)
	}

	return nil
}

type fakeIterator struct {
	addrs []address
	index int
}

func (i *fakeIterator) HasNext() bool {
	return i.index < len(i.addrs)
}

func (i *fakeIterator) GetNext() mino.Address {
	a := i.addrs[i.index]
	i.index++
	return a
}

type fakeMembership struct {
	mino.Players
	addrs []address
}

func (m fakeMembership) AddressIterator() mino.AddressIterator {
	return &fakeIterator{
		addrs: m.addrs,
	}
}

func (m fakeMembership) Len() int {
	return len(m.addrs)
}

// Handler:
// implements a handler where the stream sends back the message with its id
type testRingHandler struct {
	mino.UnsupportedHandler
	addrID   mino.Address
	neighbor mino.Address
}

func (t testRingHandler) Stream(out mino.Sender, in mino.Receiver) error {
	fromAddr, msg, err := in.Recv(context.Background())
	if err != nil {
		return xerrors.Errorf("failed to receive message from '%s': %v",
			fromAddr, err)
	}

	dummy, ok := msg.(*wrappers.StringValue)
	if !ok {
		return xerrors.Errorf("failed to cast to dummy string: %v (type %T)",
			msg, msg)
	}

	stringMsg := dummy.Value + "_" + t.addrID.String()

	dummyReturn := &wrappers.StringValue{Value: stringMsg}
	var toAddr mino.Address
	// If I am at the end of the ring I send the message to myself, which will
	// be relay by the orchestrator.
	if t.neighbor == nil {
		toAddr = address{orchestratorAddr}
	} else {
		toAddr = t.neighbor
	}

	errs := out.Send(dummyReturn, toAddr)
	err, more := <-errs
	if more {
		return xerrors.Errorf("got an error sending from the relay to the "+
			"neighbor: %v", err)
	}
	return nil
}

// Create server

func createServer(t *testing.T, id string) (string, *address, *Server, Peer) {
	identifier := id
	addr := &address{
		id: identifier,
	}
	server, err := NewServer(addr, TreeRoutingFactory)
	require.NoError(t, err)
	server.traffic.log = true
	server.StartServer()
	peer := Peer{
		Address:     server.listener.Addr().String(),
		Certificate: server.cert.Leaf,
	}

	return identifier, addr, server, peer
}
