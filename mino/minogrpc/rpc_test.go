package minogrpc

import (
	"testing"

	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/mino"
)

func TestSender(t *testing.T) {
	identifier := "1234"
	// srv := MakeMinoGrpc(identifier)

	sender := Sender{
		overlay: &UnimplementedOverlayServer{},
	}
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

	err = sender.Send(&msg, &addr)
	require.NoError(t, err)
}
