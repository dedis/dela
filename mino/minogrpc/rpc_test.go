package minogrpc

import (
	fmt "fmt"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/mino"
	grpc "google.golang.org/grpc"
)

func TestSender(t *testing.T) {
	identifier := "127.0.0.1:2000"
	// srv := MakeMinoGrpc(identifier)

	addr := mino.Address{
		Id: identifier,
	}

	m, err := ptypes.MarshalAny(&addr)
	require.NoError(t, err)

	msg := mino.Envelope{
		From:    &addr,
		To:      []*mino.Address{&addr},
		Message: m,
	}

	cert, err := makeCertificate()
	require.NoError(t, err)

	srv := grpc.NewServer()

	overlay := GrpcRPC{
		Server:     srv,
		cert:       cert,
		addr:       identifier,
		listener:   nil,
		StartChan:  make(chan struct{}),
		neighbours: make(map[string]Peer),
	}

	RegisterOverlayServer(srv, &overlayService{GrpcRPC: &overlay})

	go func() {
		err := overlay.Serve()
		require.NoError(t, err)
	}()

	<-overlay.StartChan

	peer := Peer{
		Address:     overlay.listener.Addr().String(),
		Certificate: overlay.cert.Leaf,
	}
	overlay.neighbours[identifier] = peer

	respChan, errChan := overlay.Call(&msg, &addr)
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
			fmt.Println("response: ", resp)
		case <-time.After(60 * time.Second):
			break loop
		}
	}

}
