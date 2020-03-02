package minogrpc

import (
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/mino"
	grpc "google.golang.org/grpc"
)

func TestSender(t *testing.T) {
	identifier := "1234"
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
		neighbours: make(map[string]Peer),
	}

	_, errChan := overlay.Call(&msg, &addr)
loop:
	for {
		select {
		case msgErr := <-errChan:
			t.Errorf("unexpected error: %v", msgErr)
		case <-time.After(time.Second):
			break loop
		}
	}

}
