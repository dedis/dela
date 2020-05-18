package minogrpc

import (
	"context"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minogrpc/routing"
)

func TestIntegration_BasicLifecycle(t *testing.T) {
	mm, rpcs := makeInstances(t, 10)

	authority := fake.NewAuthorityFromMino(fake.NewSigner, mm...)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, recv, err := rpcs[0].Stream(ctx, authority)
	require.NoError(t, err)

	iter := authority.AddressIterator()
	for iter.HasNext() {
		to := iter.GetNext()
		err := <-sender.Send(&empty.Empty{}, to)
		require.NoError(t, err)

		from, msg, err := recv.Recv(context.Background())
		require.NoError(t, err)
		require.Equal(t, to, from)
		require.IsType(t, (*empty.Empty)(nil), msg)
	}

	// Start the shutdown procedure.
	cancel()

	for _, m := range mm {
		// This makes sure that the relay handlers have been closed by the
		// context.
		m.(*Minogrpc).GracefulClose()
	}
}

// -----------------------------------------------------------------------------
// Utility functions

func makeInstances(t *testing.T, n int) ([]mino.Mino, []mino.RPC) {
	rtingFactory := routing.NewTreeRoutingFactory(3, AddressFactory{})
	mm := make([]mino.Mino, n)
	rpcs := make([]mino.RPC, n)
	for i := range mm {
		m, err := NewMinogrpc("127.0.0.1", 3000+uint16(i), rtingFactory)
		require.NoError(t, err)

		rpc, err := m.MakeRPC("test", testStreamHandler{})
		require.NoError(t, err)

		mm[i] = m
		rpcs[i] = rpc

		for _, k := range mm[:i] {
			km := k.(*Minogrpc)

			m.AddCertificate(k.GetAddress(), km.GetCertificate())
			km.AddCertificate(m.GetAddress(), m.GetCertificate())
		}
	}

	return mm, rpcs
}

type testStreamHandler struct {
	mino.UnsupportedHandler
}

// Stream implements mino.Handler. It implements a simple receiver that will
// return the message received and close.
func (h testStreamHandler) Stream(out mino.Sender, in mino.Receiver) error {
	from, msg, err := in.Recv(context.Background())
	if err != nil {
		return err
	}

	err = <-out.Send(msg, from)
	if err != nil {
		return err
	}

	return nil
}
