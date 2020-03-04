package minogrpc

import (
	"testing"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/mino"
	grpc "google.golang.org/grpc"
)

func Test_SimpleCall(t *testing.T) {
	identifier := "127.0.0.1:2000"

	addr := &mino.Address{
		Id: identifier,
	}

	m, err := ptypes.MarshalAny(addr)
	require.NoError(t, err)

	msg := mino.Envelope{
		From:    addr,
		To:      []*mino.Address{addr},
		Message: m,
	}

	cert, err := makeCertificate()
	require.NoError(t, err)

	srv := grpc.NewServer()

	server := Server{
		grpcSrv:    srv,
		cert:       cert,
		addr:       addr,
		listener:   nil,
		StartChan:  make(chan struct{}),
		neighbours: make(map[string]Peer),
		handlers:   make(map[string]mino.Handler),
	}

	RegisterOverlayServer(srv, &overlayService{handlers: server.handlers})

	go func() {
		err := server.Serve()
		require.NoError(t, err)
	}()

	<-server.StartChan

	peer := Peer{
		Address:     server.listener.Addr().String(),
		Certificate: server.cert.Leaf,
	}
	server.neighbours[identifier] = peer

	handler := testHandler{}
	uri := "blabla"
	rpc := RPC{
		handler: handler,
		srv:     server,
		URI:     uri,
	}

	server.handlers[uri] = handler

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

			anyResp, ok := resp.(*any.Any)
			if !ok {
				t.Error("failed to cast")
				break loop
			}
			msg2 := &mino.Envelope{}
			err = ptypes.UnmarshalAny(anyResp, msg2)
			require.NoError(t, err)

			require.Equal(t, msg.From.Id, msg2.From.Id)
		case <-time.After(2 * time.Second):
			break loop
		}
	}

}

// implements the Handler interface
type testHandler struct {
}

func (t testHandler) Process(req proto.Message) (resp proto.Message, err error) {
	return req, nil
}

func (t testHandler) Combine(req []proto.Message) (resp []proto.Message, err error) {
	return nil, nil
}

func (t testHandler) Stream(in mino.Sender, out mino.Receiver) error {
	return nil
}
