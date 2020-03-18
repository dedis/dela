package minogrpc

import (
	context "context"
	"errors"
	"strings"
	"testing"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

func Test_CreateServer(t *testing.T) {
	// Using an empty address should yield an error
	addr := address{}
	_, err := CreateServer(addr)
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

func TestServer_GetConnection(t *testing.T) {
	addr := &address{
		id: "127.0.0.1:2000",
	}

	server, err := CreateServer(addr)
	require.NoError(t, err)
	server.StartServer()

	// An empty address should yield an error
	_, err = server.getConnection("")
	require.EqualError(t, err, "empty address is not allowed")

	server.grpcSrv.GracefulStop()
}

// Use a single node to make a call that just sends back the same message.
func TestRPC_SingleSimple_Call(t *testing.T) {
	identifier := "127.0.0.1:2000"

	addr := address{
		id: identifier,
	}

	server, err := CreateServer(addr)
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

	respChan, errChan := rpc.Call(msg, fakeMembership{addrs: []address{addr}})
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

	server, err := CreateServer(addr)
	require.NoError(t, err)
	server.StartServer()

	handler := testSameHandler{time.Millisecond * 200}
	uri := "blabla"
	rpc := RPC{
		handler: handler,
		srv:     *server,
		uri:     uri,
	}

	// Using a wrong request message (nil) should yield an error while decoding
	respChan, errChan := rpc.Call(nil, &fakePlayers{players: []address{addr}})
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
	respChan, errChan = rpc.Call(msg, &fakePlayers{players: []address{addr}})
loop2:
	for {
		select {
		case msgErr := <-errChan:
			require.EqualError(t, msgErr, "failed to get client conn for '127.0.0.1:2000': couldn't find neighbour [127.0.0.1:2000]")
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

	server, err := CreateServer(addr)
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
		handler: handler,
		srv:     *server,
		uri:     uri,
	}

	server.handlers[uri] = handler

	respChan, errChan := rpc.Call(&empty.Empty{}, fakeMembership{addrs: []address{addr}})
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
	server1, err := CreateServer(addr1)
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
	server2, err := CreateServer(addr2)
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
	server3, err := CreateServer(addr3)
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
		handler: handler,
		srv:     *server1,
		uri:     uri,
	}

	server1.handlers[uri] = handler
	server2.handlers[uri] = handler
	server3.handlers[uri] = handler

	memship := fakeMembership{addrs: []address{addr1, addr2, addr3}}

	// Call the rpc on server1
	respChan, errChan := rpc.Call(&empty.Empty{}, memship)

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
	respChan, errChan = rpc.Call(&empty.Empty{}, memship)

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

	server, err := CreateServer(addr)
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

	sender, receiver := rpc.Stream(ctx, &fakePlayers{players: []address{*addr}})

	errs := sender.Send(&msg, addr)
	select {
	case err, ok := <-errs:
		if ok {
			t.Error("unexpected error from send: ", err)
		}
	}

	_, msg2, err := receiver.Recv(context.Background())
	require.NoError(t, err)

	enveloppe, ok := msg2.(*Envelope)
	require.True(t, ok)
	empty2 := &empty.Empty{}
	err = ptypes.UnmarshalAny(enveloppe.Message, empty2)
	require.NoError(t, err)

	server.grpcSrv.GracefulStop()

}

// Use a single node to make a stream that just sends back the same message.
func TestRPC_ErrorsSimple_Stream(t *testing.T) {
	identifier := "127.0.0.1:2000"

	addr := &address{
		id: identifier,
	}

	server, err := CreateServer(addr)
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
		handler: handler,
		srv:     *server,
		uri:     uri,
	}

	server.handlers[uri] = handler

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Using an empty address should yield an error
	_, receiver := rpc.Stream(ctx, &fakePlayers{players: []address{address{}}})

	_, _, err = receiver.Recv(context.Background())
	require.EqualError(t, err, "got an error from the error chan: failed to get client conn for client '': empty address is not allowed")

	server.grpcSrv.GracefulStop()

}

// Use multiple nodes to use a stream that just sends back the same message.
func TestRPC_MultipleSimple_Stream(t *testing.T) {
	identifier1 := "127.0.0.1:2001"
	addr1 := &address{
		id: identifier1,
	}
	server1, err := CreateServer(addr1)
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
	server2, err := CreateServer(addr2)
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
	server3, err := CreateServer(addr3)
	require.NoError(t, err)
	require.NoError(t, err)
	server3.StartServer()
	peer3 := Peer{
		Address:     server3.listener.Addr().String(),
		Certificate: server3.cert.Leaf,
	}

	server1.neighbours[identifier1] = peer1
	server1.neighbours[identifier2] = peer2
	server1.neighbours[identifier3] = peer3

	handler := testSameHandler{time.Millisecond * 900}
	uri := "blabla"
	rpc := RPC{
		handler: handler,
		srv:     *server1,
		uri:     uri,
	}

	// the handler must be registered on each server. Fron the client side, that
	// means the "registerNamespace" and "makeRPC" must be called on each
	// server.
	server1.handlers[uri] = handler
	server2.handlers[uri] = handler
	server3.handlers[uri] = handler

	m, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)

	msg := Envelope{
		From:    addr1.String(),
		To:      []string{addr1.String()},
		Message: m,
	}

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
	errs := sender.Send(&msg, addr1)
	select {
	case err, ok := <-errs:
		if ok {
			t.Error("unexpected error from send: ", err)
		}
	}

	_, msg2, err := rcvr.Recv(context.Background())
	require.NoError(t, err)

	enveloppe, ok := msg2.(*Envelope)
	require.True(t, ok)
	empty2 := &empty.Empty{}
	err = ptypes.UnmarshalAny(enveloppe.Message, empty2)
	require.NoError(t, err)

	// sending to three servers
	errs = sender.Send(&msg, addr1, addr2, addr3)
	select {
	case err, ok := <-errs:
		if ok {
			t.Error("unexpected error from send: ", err)
		}
	}

	// we should get three responses
	for i := 0; i < 3; i++ {
		_, msg2, err := rcvr.Recv(context.Background())
		require.NoError(t, err)
		enveloppe, ok := msg2.(*Envelope)
		require.True(t, ok)
		empty2 := &empty.Empty{}
		err = ptypes.UnmarshalAny(enveloppe.Message, empty2)
		require.NoError(t, err)
	}

	// TODO: investigate why GracefullStop yields an error
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
	server1, err := CreateServer(addr1)
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
	server2, err := CreateServer(addr2)
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
	server3, err := CreateServer(addr3)
	require.NoError(t, err)
	server3.StartServer()
	peer3 := Peer{
		Address:     server3.listener.Addr().String(),
		Certificate: server3.cert.Leaf,
	}

	server1.neighbours[identifier1] = peer1
	server1.neighbours[identifier2] = peer2
	server1.neighbours[identifier3] = peer3

	handler := testModifyHandler{}
	uri := "blabla"
	rpc := RPC{
		handler: handler,
		srv:     *server1,
		uri:     uri,
	}

	// the handler must be registered on each server. Fron the client side, that
	// means the "registerNamespace" and "makeRPC" must be called on each
	// server.
	server1.handlers[uri] = handler
	server2.handlers[uri] = handler
	server3.handlers[uri] = handler

	dummyMsg := &wrappers.StringValue{Value: "dummy_value"}
	m, err := ptypes.MarshalAny(dummyMsg)
	require.NoError(t, err)

	msg := Envelope{
		From:    addr1.String(),
		To:      []string{addr1.String()},
		Message: m,
	}

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

	// sending two messages, we should get one from each server
	errs := sender.Send(&msg, addr1, addr2, addr3)
	select {
	case err, ok := <-errs:
		if ok {
			t.Error("unexpected error from send: ", err)
		}
	}

	errs = sender.Send(&msg, addr1, addr2, addr3)
	select {
	case err, ok := <-errs:
		if ok {
			t.Error("unexpected error from send: ", err)
		}
	}

	for i := 0; i < 3; i++ {
		_, msg2, err := rcvr.Recv(context.Background())
		require.NoError(t, err)

		enveloppe, ok := msg2.(*Envelope)
		require.True(t, ok)
		dummyMsg2 := &wrappers.StringValue{}
		err = ptypes.UnmarshalAny(enveloppe.Message, dummyMsg2)
		require.NoError(t, err)
		require.Equal(t, dummyMsg.Value+dummyMsg.Value, dummyMsg2.Value)
	}

	// TODO: investigate why GracefullStop yields an error
	server1.grpcSrv.GracefulStop()
	server2.grpcSrv.GracefulStop()
	server3.grpcSrv.GracefulStop()
	require.NoError(t, err)
}

func TestSender_Send(t *testing.T) {
	sender := sender{
		participants: make([]player, 0),
	}

	// sending to an empty list should not yield an error
	errs := sender.Send(nil)
	select {
	case err, ok := <-errs:
		if ok {
			t.Error("unexpected error from send: ", err)
		}
	}

	// giving an empty address should add an error since it won't be found in the list of
	// participants
	addr := address{}
	errs = sender.Send(nil, addr)
	select {
	case err, ok := <-errs:
		if !ok {
			t.Error("there should be an error")
		}
		require.EqualError(t, err, xerrors.Errorf("participant '%s' not in the list", addr).Error())
	}

	// now I add the participant to the list, an error should be given since the
	// message is nil
	addr = address{id: "fake"}
	sender.participants = append(sender.participants, player{
		address: addr,
	})
	errs = sender.Send(nil, addr)
	select {
	case err, ok := <-errs:
		if !ok {
			t.Error("there should be an error")
		}
		require.EqualError(t, err, encoding.NewAnyEncodingError(nil, errors.New("proto: Marshal called with nil")).Error())
	}
}

func TestReceiver_Recv(t *testing.T) {
	receiver := receiver{
		errs: make(chan error, 1),
		in:   make(chan *OverlayMsg, 1),
	}

	// If there is a wrong message (nil), then it should output an error
	receiver.in <- nil
	_, _, err := receiver.Recv(context.Background())
	require.EqualError(t, err, "message is nil")

	// now with a non nil message, but its content is nil
	msg := &OverlayMsg{
		Message: nil,
	}
	receiver.in <- msg
	_, _, err = receiver.Recv(context.Background())
	require.EqualError(t, err, encoding.NewAnyDecodingError(msg.Message, errors.New("message is nil")).Error())
}

// -----------------
// Utility functions

// implements a handler interface that just returns the input
type testSameHandler struct {
	timeout time.Duration
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
		select {
		case err, ok := <-errs:
			if ok {
				return xerrors.Errorf("failed to send message to the sender: %v", err)
			}
		}
	}
}

// implements a handler interface that receives an address and adds a suffix to
// it (for the call) or aggregate all the address (for the stream). The stream
// expects 3 calls before returning the aggregate addresses.
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

		enveloppe, ok := msg.(*Envelope)
		if !ok {
			return xerrors.New("failed to cast message to envelope")
		}

		dummy := &wrappers.StringValue{}
		err = ptypes.UnmarshalAny(enveloppe.Message, dummy)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal dummy message: %v", err)
		}

		dummyMsg += dummy.Value
	}
	dummyReturn := &wrappers.StringValue{Value: dummyMsg}
	anyDummy, err := ptypes.MarshalAny(dummyReturn)
	if err != nil {
		return xerrors.Errorf("failed to marshal any: %v", err)
	}

	errs := out.Send(&Envelope{Message: anyDummy}, addr)
	select {
	case err, ok := <-errs:
		if ok {
			return xerrors.Errorf("failed to send message to the sender: %v", err)
		}
	}

	return nil
}

type fakeIterator struct {
	addrs []address
	index int
}

func (i *fakeIterator) HasNext() bool {
	if i.index < len(i.addrs) {
		return true
	}
	return false
}

func (i *fakeIterator) GetNext() mino.Address {
	a := i.addrs[i.index]
	i.index++
	return a
}

type fakeMembership struct {
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
