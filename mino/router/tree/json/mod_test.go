package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino/router/tree/types"
	"go.dedis.ch/dela/serde"
)

func TestPacketFormat_Encode(t *testing.T) {
	fmt := packetFormat{}

	ctx := fake.NewContext()
	pkt := types.NewPacket(fake.NewAddress(0), []byte("data"), fake.NewAddress(1))

	data, err := fmt.Encode(ctx, pkt)
	require.NoError(t, err)
	require.Equal(t, `{"Source":"AAAAAA==","Dest":["AQAAAA=="],"Message":"ZGF0YQ=="}`, string(data))

	_, err = fmt.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message 'fake.Message'")

	_, err = fmt.Encode(ctx, types.NewPacket(fake.NewBadAddress(), nil))
	require.EqualError(t, err, fake.Err("failed to marshal source addr"))

	_, err = fmt.Encode(ctx, types.NewPacket(fake.NewAddress(0), nil, fake.NewBadAddress()))
	require.EqualError(t, err, fake.Err("failed to marshal dest addr"))

	_, err = fmt.Encode(fake.NewBadContext(), pkt)
	require.EqualError(t, err, fake.Err("failed to marshal packet"))
}

func TestPacketFormat_Decode(t *testing.T) {
	fmt := packetFormat{}

	pkt := types.NewPacket(fake.NewAddress(0), []byte{}, fake.NewAddress(1))

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, types.AddrKey{}, fake.AddressFactory{})

	msg, err := fmt.Decode(ctx, []byte(`{"Message":"","Dest":["AQAAAA=="]}`))
	require.NoError(t, err)
	require.Equal(t, pkt, msg)

	_, err = fmt.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("failed to unmarshal packet"))

	badCtx := serde.WithFactory(ctx, types.AddrKey{}, nil)
	_, err = fmt.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "invalid address factory '<nil>'")
}

func TestHandshakeFormat_Encode(t *testing.T) {
	fmt := hsFormat{}

	ctx := fake.NewContext()
	hs := types.NewHandshake(5, fake.NewAddress(1))

	data, err := fmt.Encode(ctx, hs)
	require.NoError(t, err)
	require.Equal(t, `{"Height":5,"Addresses":["AQAAAA=="]}`, string(data))

	_, err = fmt.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message 'fake.Message'")

	_, err = fmt.Encode(ctx, types.NewHandshake(1, fake.NewBadAddress()))
	require.EqualError(t, err, fake.Err("failed to marshal address"))

	_, err = fmt.Encode(fake.NewBadContext(), hs)
	require.EqualError(t, err, fake.Err("failed to marshal handshake"))
}

func TestHandshakeFormat_Decode(t *testing.T) {
	fmt := hsFormat{}

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, types.AddrKey{}, fake.AddressFactory{})

	msg, err := fmt.Decode(ctx, []byte(`{"Addresses":["AQAAAA=="]}`))
	require.NoError(t, err)
	require.Equal(t, types.NewHandshake(0, fake.NewAddress(1)), msg)

	_, err = fmt.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("failed to unmarshal"))

	badCtx := serde.WithFactory(ctx, types.AddrKey{}, nil)
	_, err = fmt.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "invalid address factory '<nil>'")
}
