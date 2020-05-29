package certs

import (
	"bytes"
	"crypto/tls"
	"sync"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

// InMemoryStore is a certificate store that keeps the certificates in
// memory only, which means it does not persist.
//
// - implements certs.Store
type InMemoryStore struct {
	certs       *sync.Map
	hashFactory crypto.HashFactory
}

// NewInMemoryStore creates a new empty certificate store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		certs:       &sync.Map{},
		hashFactory: crypto.NewSha256Factory(),
	}
}

// Store implements certs.Store.
func (s *InMemoryStore) Store(addr mino.Address, cert *tls.Certificate) {
	s.certs.Store(addr, cert)
}

// Load implements certs.Store.
func (s *InMemoryStore) Load(addr mino.Address) *tls.Certificate {
	val, found := s.certs.Load(addr)
	if !found {
		return nil
	}
	return val.(*tls.Certificate)
}

// Delete implements certs.Store.
func (s *InMemoryStore) Delete(addr mino.Address) {
	s.certs.Delete(addr)
}

// Range implements certs.Store.
func (s *InMemoryStore) Range(fn func(addr mino.Address, cert *tls.Certificate) bool) {
	s.certs.Range(func(key, value interface{}) bool {
		return fn(key.(mino.Address), value.(*tls.Certificate))
	})
}

// Fetch implements certs.Store.
func (s *InMemoryStore) Fetch(addr Dialable, hash []byte) error {
	cfg := &tls.Config{
		// The server certificate is unknown yet, but we don't want to
		// communicate, only fetch the certificate. The integrity is verified
		// through the hash to prevent man-in-the-middle attacks.
		InsecureSkipVerify: true,
	}

	// This connection will be used to fetch the certificate of the server and
	// to verify that it matches the expected hash.
	conn, err := tls.Dial("tcp", addr.GetDialAddress(), cfg)
	if err != nil {
		return xerrors.Errorf("failed to dial: %v", err)
	}

	conn.Close()

	peers := conn.ConnectionState().PeerCertificates
	// Server certificate should be self-signed and thus a chain of length 1.
	if len(peers) != 1 {
		return xerrors.Errorf("expect exactly one certificate but found %d", len(peers))
	}

	cert := &tls.Certificate{Leaf: peers[0]}

	digest, err := s.Hash(cert)
	if err != nil {
		return xerrors.Errorf("couldn't hash certificate: %v", err)
	}

	if !bytes.Equal(digest, hash) {
		return xerrors.Errorf("mismatch certificate digest")
	}

	s.certs.Store(addr, cert)

	return nil
}

// Hash implements certs.Store.
func (s *InMemoryStore) Hash(cert *tls.Certificate) ([]byte, error) {
	h := s.hashFactory.New()

	_, err := h.Write(cert.Leaf.Raw)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write leaf: %v", err)
	}

	return h.Sum(nil), nil
}
