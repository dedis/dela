package minows

import (
	"crypto/rand"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func TestNewMinows(t *testing.T) {
	const listen = "/ip4/0.0.0.0/tcp/7452/ws"
	const ws = "/ip4/127.0.0.1/tcp/7452/ws"
	const wss = "/ip4/127.0.0.1/tcp/443/wss"
	var tests = map[string]struct {
		listen string
		public string
	}{
		"ws":  {listen: listen, public: ws},
		"wss": {listen: listen, public: wss},
	}
	key := mustCreateKey(t)
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			listen := mustCreateMultiaddress(t, tt.listen)
			public := mustCreateMultiaddress(t, tt.public)

			m, err := NewMinows(listen, public, key)
			require.NoError(t, err)
			require.NotNil(t, m)
			require.IsType(t, &Minows{}, m)
			require.NoError(t, m.(*Minows).stop())
		})
	}
}

func TestNewMinows_OptionalPublic(t *testing.T) {
	listen := mustCreateMultiaddress(t, "/ip4/0.0.0.0/tcp/7452/ws")
	random := mustCreateMultiaddress(t, "/ip4/127.0.0.1/tcp/0/ws")
	tests := map[string]ma.Multiaddr{
		"no public":     listen,
		"random listen": random,
	}
	key := mustCreateKey(t)

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := NewMinows(tt, nil, key)
			require.NoError(t, err)
			require.NotNil(t, m)
			require.IsType(t, &Minows{}, m)
			require.NoError(t, m.(*Minows).stop())
		})

	}
}

func Test_minows_GetAddressFactory(t *testing.T) {
	const listen = "/ip4/0.0.0.0/tcp/7452"
	const ws = "/ip4/127.0.0.1/tcp/7452/ws"
	const wss = "/ip4/127.0.0.1/tcp/443/wss"
	type m struct {
		listen string
		public string
	}
	tests := map[string]struct {
		m m
	}{
		"ws":  {m{listen, ws}},
		"wss": {m{listen, wss}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m, stop := mustCreateMinows(t, tt.m.listen, tt.m.public)
			defer stop()

			factory := m.GetAddressFactory()
			require.NotNil(t, factory)
			require.IsType(t, addressFactory{}, factory)
		})
	}
}

func Test_minows_GetAddress(t *testing.T) {
	const listen = "/ip4/127.0.0.1/tcp/7452"
	const publicWS = "/ip4/127.0.0.1/tcp/80/ws"
	const wss = "/ip4/127.0.0.1/tcp/443/wss"
	key := mustCreateKey(t)
	id := mustDerivePeerID(t, key).String()
	type m struct {
		listen string
		public string
		key    crypto.PrivKey
	}
	type want struct {
		location string
		identity string
	}
	tests := map[string]struct {
		m    m
		want want
	}{
		"ws":        {m{listen, publicWS, key}, want{publicWS, id}},
		"wss":       {m{listen, wss, key}, want{wss, id}},
		"no public": {m{listen, "", key}, want{listen, id}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := NewMinows(mustCreateMultiaddress(t, tt.m.listen),
				mustCreateMultiaddress(t, tt.m.public), tt.m.key)
			require.NoError(t, err)
			defer require.NoError(t, m.(*Minows).stop())
			want := mustCreateAddress(t, tt.want.location, tt.want.identity)

			got := m.GetAddress()
			require.Equal(t, want, got)
		})
	}
}

func Test_minows_GetAddress_Random(t *testing.T) {
	random := "/ip4/127.0.0.1/tcp/0/ws"
	listen := mustCreateMultiaddress(t, random)
	key := mustCreateKey(t)
	m, err := NewMinows(listen, nil, key)
	require.NoError(t, err)
	defer require.NoError(t, m.(*Minows).stop())

	got := m.GetAddress().(address)
	port, err := got.location.ValueForProtocol(ma.P_TCP)
	require.NoError(t, err)
	require.NotEqual(t, 0, port)
}

func Test_minows_WithSegment_Empty(t *testing.T) {
	const listen = "/ip4/0.0.0.0/tcp/7452"
	const ws = "/ip4/127.0.0.1/tcp/7452/ws"
	m, stop := mustCreateMinows(t, listen, ws)
	defer stop()

	got := m.WithSegment("")
	require.Equal(t, m, got)
}

func Test_minows_WithSegment(t *testing.T) {
	const listen = "/ip4/0.0.0.0/tcp/7452"
	const ws = "/ip4/127.0.0.1/tcp/7452/ws"
	m, stop := mustCreateMinows(t, listen, ws)
	defer stop()

	got := m.WithSegment("test")
	require.NotEqual(t, m, got)

	got2 := m.WithSegment("test").WithSegment("test")
	require.NotEqual(t, m, got2)
	require.NotEqual(t, got, got2)
}

func Test_minows_CreateRPC_InvalidName(t *testing.T) {
	const listen = "/ip4/0.0.0.0/tcp/7452"
	const ws = "/ip4/127.0.0.1/tcp/7452/ws"
	m, stop := mustCreateMinows(t, listen, ws)
	defer stop()

	_, err := m.CreateRPC("invalid name", nil, nil)
	require.Error(t, err)
}

func Test_minows_CreateRPC_AlreadyExists(t *testing.T) {
	const listen = "/ip4/0.0.0.0/tcp/7452"
	const ws = "/ip4/127.0.0.1/tcp/7452/ws"
	m, stop := mustCreateMinows(t, listen, ws)
	defer stop()

	_, err := m.CreateRPC("test", nil, nil)
	require.NoError(t, err)
	_, err = m.CreateRPC("test", nil, nil)
	require.Error(t, err)
}

func Test_minows_CreateRPC_InvalidSegment(t *testing.T) {
	const listen = "/ip4/0.0.0.0/tcp/7452"
	const ws = "/ip4/127.0.0.1/tcp/7452/ws"
	m, stop := mustCreateMinows(t, listen, ws)
	defer stop()
	m = m.WithSegment("invalid segment").(*Minows)

	_, err := m.CreateRPC("test", nil, nil)
	require.Error(t, err)
}

func Test_minows_CreateRPC(t *testing.T) {
	const listen = "/ip4/0.0.0.0/tcp/7452"
	const ws = "/ip4/127.0.0.1/tcp/7452/ws"
	m, stop := mustCreateMinows(t, listen, ws)
	defer stop()

	r1, err := m.CreateRPC("test", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, r1)
	r2, err := m.CreateRPC("Test", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, r2)

	m = m.WithSegment("segment").(*Minows)
	r3, err := m.CreateRPC("test", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, r3)
	r4, err := m.CreateRPC("Test", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, r4)
}

func mustCreateMinows(t *testing.T, listen string, public string) (
	*Minows,
	func(),
) {
	key := mustCreateKey(t)
	lis := mustCreateMultiaddress(t, listen)
	pub := mustCreateMultiaddress(t, public)
	m, err := NewMinows(lis, pub, key)
	require.NoError(t, err)
	ws := m.(*Minows)
	stop := func() { require.NoError(t, ws.stop()) }
	return ws, stop
}

func mustCreateKey(t *testing.T) crypto.PrivKey {
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	require.NoError(t, err)
	return key
}

func mustDerivePeerID(t *testing.T, key crypto.PrivKey) peer.ID {
	peerId, err := peer.IDFromPrivateKey(key)
	require.NoError(t, err)
	return peerId
}
