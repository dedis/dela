// This file contains the implementation of an in-memory certificate storage.
//
// Documentation Last Review: 07.10.2020
//

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
// - implements certs.Storage
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

// Store implements certs.Storage. It stores the certificate with the address as
// the key.
func (s *InMemoryStore) Store(addr mino.Address, chain CertChain) error {
	s.certs.Store(addr, chain)

	return nil
}

// Load implements certs.Storage. It looks for the certificate associated to the
// address. If it does not exist, it will return nil.
func (s *InMemoryStore) Load(addr mino.Address) (CertChain, error) {
	val, found := s.certs.Load(addr)
	if !found {
		return nil, nil
	}

	return val.(CertChain), nil
}

// Delete implements certs.Storage. It deletes the certificate associated to the
// address if any, otherwise it does nothing.
func (s *InMemoryStore) Delete(addr mino.Address) error {
	s.certs.Delete(addr)

	return nil
}

// Range implements certs.Storage. It iterates over all the certificates stored
// as long as the callback return true.
func (s *InMemoryStore) Range(fn func(addr mino.Address, chain CertChain) bool) error {
	s.certs.Range(func(key, value interface{}) bool {
		return fn(key.(mino.Address), value.(CertChain))
	})

	return nil
}

// Fetch implements certs.Storage. It tries to open a TLS connection to the
// address only to get the certificate from the distant peer. The connection is
// dropped right after the certificate is read and stored.
func (s *InMemoryStore) Fetch(addr Dialable, hash []byte) error {
	cfg := &tls.Config{
		// The server certificate is unknown yet, but we don't want to
		// communicate, only fetch the certificate. The integrity is verified
		// through the hash to prevent man-in-the-middle attacks.
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}

	// This connection will be used to fetch the certificate of the server and
	// to verify that it matches the expected hash.
	conn, err := tls.Dial("tcp", addr.GetDialAddress(), cfg)
	if err != nil {
		return xerrors.Errorf("failed to dial: %v", err)
	}

	conn.Close()

	// we can assume that `peers` is not empty (see doc of `PeerCertificates`)
	peers := conn.ConnectionState().PeerCertificates

	chain := bytes.Buffer{}

	for _, peer := range peers {
		chain.Write(peer.Raw)
	}

	digest, err := s.Hash(chain.Bytes())
	if err != nil {
		return xerrors.Errorf("couldn't hash certificate: %v", err)
	}

	if !bytes.Equal(digest, hash) {
		return xerrors.Errorf("mismatch certificate digest")
	}

	// We need only the root certificate from a distant peer, as it is
	// sufficient to verify it.
	s.certs.Store(addr, CertChain(peers[len(peers)-1].Raw))

	return nil
}

// Hash implements certs.Storage. It returns the unique digest for the
// certificate.
func (s *InMemoryStore) Hash(chain CertChain) ([]byte, error) {
	h := s.hashFactory.New()

	_, err := h.Write(chain)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write cert: %v", err)
	}

	return h.Sum(nil), nil
}
