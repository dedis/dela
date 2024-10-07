package minows

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-yamux/v4"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/testing/fake"
)

func Test_session_Send(t *testing.T) {
	handler := newEchoHandler()
	const addrInitiator = "/ip4/127.0.0.1/tcp/6001/ws"
	initiator, stop := mustCreateMinows(t, addrInitiator, addrInitiator)
	defer stop()
	r := mustCreateRPC(t, initiator, "test", handler)

	const addrPlayer1 = "/ip4/127.0.0.1/tcp/6002/ws"
	player1, stop := mustCreateMinows(t, addrPlayer1, addrPlayer1)
	defer stop()
	mustCreateRPC(t, player1, "test", handler)

	const addrPlayer2 = "/ip4/127.0.0.1/tcp/6003/ws"
	player2, stop := mustCreateMinows(t, addrPlayer2, addrPlayer2)
	defer stop()
	mustCreateRPC(t, player2, "test", handler)

	s, _, stop := mustStream(t, r, player1, player2)
	defer stop()

	errs := s.Send(fake.Message{}, player1.GetAddress())
	err, open := <-errs
	require.NoError(t, err)
	require.False(t, open)

	errs = s.Send(fake.Message{}, player1.GetAddress(), player2.GetAddress())
	err, open = <-errs
	require.NoError(t, err)
	require.False(t, open)

	handler.wait(3)
	require.Equal(t, []mino.Address{s.(*messageHandler).myAddr,
		s.(*messageHandler).myAddr, s.(*messageHandler).myAddr}, handler.from)
	require.Equal(t, []serde.Message{fake.Message{},
		fake.Message{}, fake.Message{}}, handler.messages)
}

func Test_session_Send_ToSelf(t *testing.T) {
	handler := newEchoHandler()
	const addrInitiator = "/ip4/127.0.0.1/tcp/6001/ws"
	initiator, stop := mustCreateMinows(t, addrInitiator, addrInitiator)
	defer stop()
	r := mustCreateRPC(t, initiator, "test", handler)

	const addrPlayer1 = "/ip4/127.0.0.1/tcp/6002/ws"
	player1, stop := mustCreateMinows(t, addrPlayer1, addrPlayer1)
	defer stop()
	mustCreateRPC(t, player1, "test", handler)

	s, _, stop := mustStream(t, r, initiator, player1)
	defer stop()

	errs := s.Send(fake.Message{}, initiator.GetAddress())
	err, open := <-errs
	require.NoError(t, err)
	require.False(t, open)

	errs = s.Send(fake.Message{}, initiator.GetAddress(), player1.GetAddress())
	err, open = <-errs
	require.NoError(t, err)
	require.False(t, open)

	handler.wait(3)
	require.Equal(t, []mino.Address{s.(*messageHandler).myAddr,
		s.(*messageHandler).myAddr, s.(*messageHandler).myAddr}, handler.from)
	require.Equal(t, []serde.Message{fake.Message{},
		fake.Message{}, fake.Message{}}, handler.messages)
	require.NotEqual(t, s.(*messageHandler).myAddr, initiator.GetAddress())
}

func Test_session_Send_WrongAddressType(t *testing.T) {
	handler := newEchoHandler()
	const addrInitiator = "/ip4/127.0.0.1/tcp/6001/ws"
	initiator, stop := mustCreateMinows(t, addrInitiator, addrInitiator)
	defer stop()
	r := mustCreateRPC(t, initiator, "test", handler)

	const addrPlayer = "/ip4/127.0.0.1/tcp/6002/ws"
	player, stop := mustCreateMinows(t, addrPlayer, addrPlayer)
	defer stop()
	mustCreateRPC(t, player, "test", handler)

	s, _, stop := mustStream(t, r, player)
	defer stop()

	errs := s.Send(fake.Message{}, fake.Address{})
	require.ErrorContains(t, <-errs, "wrong address type")
}

func Test_session_Send_AddressNotPlayer(t *testing.T) {
	handler := newEchoHandler()
	const addrInitiator = "/ip4/127.0.0.1/tcp/6001/ws"
	initiator, stop := mustCreateMinows(t, addrInitiator, addrInitiator)
	defer stop()
	r := mustCreateRPC(t, initiator, "test", handler)

	const addrPlayer = "/ip4/127.0.0.1/tcp/6002/ws"
	player, stop := mustCreateMinows(t, addrPlayer, addrPlayer)
	defer stop()
	mustCreateRPC(t, player, "test", handler)

	s, _, stop := mustStream(t, r, player)
	defer stop()
	notPlayer := mustCreateAddress(t, "/ip4/127.0.0.1/tcp/6003/ws",
		"QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU")

	errs := s.Send(fake.Message{}, notPlayer)
	require.ErrorContains(t, <-errs, "not player")
}

func Test_session_Send_SessionEnded(t *testing.T) {
	handler := newEchoHandler()
	const addrInitiator = "/ip4/127.0.0.1/tcp/6001/ws"
	initiator, stop := mustCreateMinows(t, addrInitiator, addrInitiator)
	defer stop()
	rpc := mustCreateRPC(t, initiator, "test", handler)

	const addrPlayer = "/ip4/127.0.0.1/tcp/6002/ws"
	player, stop := mustCreateMinows(t, addrPlayer, addrPlayer)
	defer stop()
	mustCreateRPC(t, player, "test", handler)

	s, _, stop := mustStream(t, rpc, initiator, player)
	stop()

	errs := s.Send(fake.Message{}, initiator.GetAddress(), player.GetAddress())
	for i := 0; i < 2; i++ {
		err := (<-errs).Error()
		require.True(t, strings.Contains(err, network.ErrReset.Error()) ||
			strings.Contains(err, yamux.ErrSessionShutdown.Error()))
	}

	_, open := <-errs
	require.False(t, open)
	require.Nil(t, handler.messages)
}

func Test_session_Recv(t *testing.T) {
	handler := newEchoHandler()
	const addrInitiator = "/ip4/127.0.0.1/tcp/6001/ws"
	initiator, stop := mustCreateMinows(t, addrInitiator, addrInitiator)
	defer stop()
	r := mustCreateRPC(t, initiator, "test", handler)

	const addrPlayer1 = "/ip4/127.0.0.1/tcp/6002/ws"
	player1, stop := mustCreateMinows(t, addrPlayer1, addrPlayer1)
	defer stop()
	mustCreateRPC(t, player1, "test", handler)

	const addrPlayer2 = "/ip4/127.0.0.1/tcp/6003/ws"
	player2, stop := mustCreateMinows(t, addrPlayer2, addrPlayer2)
	defer stop()
	mustCreateRPC(t, player2, "test", handler)

	sender, receiver, stop := mustStream(t, r, player1, player2)
	defer stop()

	errs := sender.Send(fake.Message{}, player1.GetAddress())
	require.NoError(t, <-errs)
	ctx, cancel := setTimeout()
	defer cancel()

	from, msg, err := receiver.Recv(ctx)
	require.NoError(t, err)
	require.Equal(t, player1.GetAddress(), from)
	require.Equal(t, fake.Message{}, msg)

	errs = sender.Send(fake.Message{},
		player1.GetAddress(), player2.GetAddress())
	require.NoError(t, <-errs)

	from1, msg, err := receiver.Recv(ctx)
	require.NoError(t, err)
	require.True(t, from1.Equal(player1.GetAddress()) ||
		from1.Equal(player2.GetAddress()))
	require.Equal(t, fake.Message{}, msg)

	from2, msg, err := receiver.Recv(ctx)
	require.NoError(t, err)
	require.True(t, from2.Equal(player1.GetAddress()) ||
		from2.Equal(player2.GetAddress()))
	require.Equal(t, fake.Message{}, msg)

	require.NotEqual(t, from1, from2)
}

func Test_session_Recv_FromSelf(t *testing.T) {
	handler := newEchoHandler()
	const addrInitiator = "/ip4/127.0.0.1/tcp/6001/ws"
	initiator, stop := mustCreateMinows(t, addrInitiator, addrInitiator)
	defer stop()
	rpc := mustCreateRPC(t, initiator, "test", handler)

	const addrPlayer1 = "/ip4/127.0.0.1/tcp/6002/ws"
	player1, stop := mustCreateMinows(t, addrPlayer1, addrPlayer1)
	defer stop()
	mustCreateRPC(t, player1, "test", handler)

	s, r, stop := mustStream(t, rpc, initiator, player1)
	defer stop()

	errs := s.Send(fake.Message{}, initiator.GetAddress())
	require.NoError(t, <-errs)

	ctx, cancel := setTimeout()
	defer cancel()

	from, msg, err := r.Recv(ctx)
	require.NoError(t, err)
	require.Equal(t, initiator.GetAddress(), from)
	require.Equal(t, fake.Message{}, msg)

	errs = s.Send(fake.Message{}, initiator.GetAddress(), player1.GetAddress())
	require.NoError(t, <-errs)

	from1, msg, err := r.Recv(ctx)
	require.NoError(t, err)
	require.True(t, from1.Equal(initiator.GetAddress()) ||
		from1.Equal(player1.GetAddress()))
	require.Equal(t, fake.Message{}, msg)

	from2, msg, err := r.Recv(ctx)
	require.NoError(t, err)
	require.True(t, from2.Equal(initiator.GetAddress()) ||
		from2.Equal(player1.GetAddress()))
	require.Equal(t, fake.Message{}, msg)

	require.NotEqual(t, from1, from2)
	require.Equal(t, []mino.Address{s.(*messageHandler).myAddr,
		s.(*messageHandler).myAddr, s.(*messageHandler).myAddr}, handler.from)
	require.NotEqual(t, s.(*messageHandler).myAddr, initiator.GetAddress())
}

func Test_session_Recv_SessionEnded(t *testing.T) {
	if testing.Short() {
		t.Skip("See issue https://github.com/dedis/dela/issues/291")
	}
	handler := newEchoHandler()
	const addrInitiator = "/ip4/127.0.0.1/tcp/6001/ws"
	initiator, stop := mustCreateMinows(t, addrInitiator, addrInitiator)
	defer stop()
	rpc := mustCreateRPC(t, initiator, "test", handler)

	const addrPlayer = "/ip4/127.0.0.1/tcp/6002/ws"
	player, stop := mustCreateMinows(t, addrPlayer, addrPlayer)
	defer stop()
	mustCreateRPC(t, player, "test", handler)

	s, r, end := mustStream(t, rpc, initiator, player)

	errs := s.Send(fake.Message{}, initiator.GetAddress(), player.GetAddress())
	_, open := <-errs
	require.False(t, open)

	end()

	ctx, cancel := setTimeout()
	defer cancel()
	_, _, err := r.Recv(ctx)
	require.ErrorIs(t, err, io.EOF)
}

func Test_session_Recv_ContextCancelled(t *testing.T) {
	handler := newEchoHandler()
	const addrInitiator = "/ip4/127.0.0.1/tcp/6001/ws"
	initiator, stop := mustCreateMinows(t, addrInitiator, addrInitiator)
	defer stop()
	r := mustCreateRPC(t, initiator, "test", handler)

	const addrPlayer = "/ip4/127.0.0.1/tcp/6002/ws"
	player, stop := mustCreateMinows(t, addrPlayer, addrPlayer)
	defer stop()
	mustCreateRPC(t, player, "test", handler)

	_, receiver, stop := mustStream(t, r, player)
	defer stop()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := receiver.Recv(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func setTimeout() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	return ctx, cancel
}

func mustStream(t *testing.T, rpc mino.RPC,
	minos ...mino.Mino) (mino.Sender, mino.Receiver, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	var addrs []mino.Address
	for _, m := range minos {
		addrs = append(addrs, m.GetAddress())
	}
	players := mino.NewAddresses(addrs...)
	end := func() {
		cancel()
	}
	s, r, err := rpc.Stream(ctx, players)
	require.NoError(t, err)
	return s, r, end
}
