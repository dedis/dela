package minows

import (
	"path/filepath"
	"testing"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/store/kv"
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

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			listen := mustCreateMultiaddress(t, tt.listen)
			public := mustCreateMultiaddress(t, tt.public)

			db, err := kv.New(filepath.Join(t.TempDir(), "minows.db"))
			require.NoError(t, err)

			m, err := NewMinows(listen, public, &db)
			require.NoError(t, err)
			require.NotNil(t, m)
			require.IsType(t, &Minows{}, m)
			require.NoError(t, m.(*Minows).Stop())
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

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			db, err := kv.New(filepath.Join(t.TempDir(), "minows.db"))
			require.NoError(t, err)

			m, err := NewMinows(tt, nil, &db)
			require.NoError(t, err)
			require.NotNil(t, m)
			require.IsType(t, &Minows{}, m)
			require.NoError(t, m.(*Minows).Stop())
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

	type m struct {
		listen string
		public string
	}
	type want struct {
		location string
	}
	tests := map[string]struct {
		m    m
		want want
	}{
		"ws":        {m{listen, publicWS}, want{publicWS}},
		"wss":       {m{listen, wss}, want{wss}},
		"no public": {m{listen, ""}, want{listen}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			db, err := kv.New(filepath.Join(t.TempDir(), "minows.db"))
			require.NoError(t, err)

			m, err := NewMinows(mustCreateMultiaddress(t, tt.m.listen),
				mustCreateMultiaddress(t, tt.m.public), &db)
			require.NoError(t, err)
			defer require.NoError(t, m.(*Minows).Stop())
			want := mustCreateAddress(t, tt.want.location, m.(*Minows).GetPeerID())

			got := m.GetAddress()
			require.Equal(t, want, got)
		})
	}
}

func Test_minows_GetAddress_Random(t *testing.T) {
	random := "/ip4/127.0.0.1/tcp/0/ws"
	listen := mustCreateMultiaddress(t, random)

	db, err := kv.New(filepath.Join(t.TempDir(), "minows.db"))
	require.NoError(t, err)

	m, err := NewMinows(listen, nil, &db)
	require.NoError(t, err)
	defer require.NoError(t, m.(*Minows).Stop())

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
	// store
	db, err := kv.New(filepath.Join(t.TempDir(), "minows.db"))
	require.NoError(t, err)

	lis := mustCreateMultiaddress(t, listen)
	pub := mustCreateMultiaddress(t, public)
	m, err := NewMinows(lis, pub, &db)
	require.NoError(t, err)
	ws := m.(*Minows)
	stop := func() { require.NoError(t, ws.Stop()) }
	return ws, stop
}
