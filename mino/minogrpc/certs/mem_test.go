package certs

import (
	"crypto/tls"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
)

func TestInMemoryStore_Store(t *testing.T) {
	store := NewInMemoryStore()

	store.Store(fake.NewAddress(0), CertChain{})
	store.Store(fake.NewAddress(1), CertChain{})
	store.Store(fake.NewAddress(0), CertChain{})

	num := 0
	store.certs.Range(func(key, value interface{}) bool {
		num++
		require.IsType(t, fake.Address{}, key)
		require.IsType(t, CertChain{}, value)
		return true
	})
	require.Equal(t, 2, num)
}

func TestInMemoryStore_Load(t *testing.T) {
	store := NewInMemoryStore()

	store.certs.Store(fake.NewAddress(0), CertChain{})
	store.certs.Store(fake.NewAddress(1), CertChain{})

	cert, err := store.Load(fake.NewAddress(0))
	require.NoError(t, err)
	require.NotNil(t, cert)

	cert, err = store.Load(fake.NewAddress(1))
	require.NoError(t, err)
	require.NotNil(t, cert)

	cert, err = store.Load(fake.NewAddress(2))
	require.NoError(t, err)
	require.Nil(t, cert)
}

func TestInMemoryStore_Delete(t *testing.T) {
	store := NewInMemoryStore()

	store.certs.Store(fake.NewAddress(0), CertChain{})
	store.certs.Store(fake.NewAddress(1), CertChain{})

	store.Delete(fake.NewAddress(0))

	_, found := store.certs.Load(fake.NewAddress(0))
	require.False(t, found)

	_, found = store.certs.Load(fake.NewAddress(1))
	require.True(t, found)
}

func TestInMemoryStore_Range(t *testing.T) {
	store := NewInMemoryStore()

	store.certs.Store(fake.NewAddress(0), CertChain{})
	store.certs.Store(fake.NewAddress(1), CertChain{})

	num := 0
	store.Range(func(addr mino.Address, chain CertChain) bool {
		require.Regexp(t, "fake.Address\\[[0-1]\\]", addr.String())
		num++
		return true
	})

	require.Equal(t, 2, num)
}

func TestInMemoryStore_Fetch(t *testing.T) {
	store := NewInMemoryStore()

	cert, certBuf := fake.MakeFullCertificate(t)

	cfg := &tls.Config{
		Certificates: []tls.Certificate{*cert},
	}

	l := listenTLS(t, cfg)
	defer l.Close()

	digest, err := store.Hash(certBuf)
	require.NoError(t, err)

	err = store.Fetch(fakeDialable{host: l.Addr().String()}, digest)
	require.NoError(t, err)
	l.Close()

	err = store.Fetch(fakeDialable{}, digest)
	require.EqualError(t, err, "failed to dial: dial tcp: missing address")

	l = listenTLS(t, cfg)
	err = store.Fetch(fakeDialable{host: l.Addr().String()}, []byte{})
	require.EqualError(t, err, "mismatch certificate digest")
	l.Close()

	l = listenTLS(t, cfg)
	store.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	err = store.Fetch(fakeDialable{host: l.Addr().String()}, []byte{})
	require.EqualError(t, err,
		fake.Err("couldn't hash certificate: couldn't write cert"))
}

func TestInMemoryStore_Hash(t *testing.T) {
	store := NewInMemoryStore()

	digest, err := store.Hash([]byte{})
	require.NoError(t, err)
	require.Len(t, digest, 32)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeDialable struct {
	mino.Address
	host string
}

func (a fakeDialable) GetDialAddress() string {
	return a.host
}

func listenTLS(t *testing.T, cfg *tls.Config) net.Listener {
	l, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	require.NoError(t, err)

	go func() {
		conn, err := l.Accept()
		require.NoError(t, err)
		require.NotNil(t, conn)

		conn.(*tls.Conn).Handshake()
		conn.Close()
	}()

	return l
}
