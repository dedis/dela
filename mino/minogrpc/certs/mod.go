// Package certs defines a certificate store that will provide primitives to
// store and get certificates for a given address. It also provide a primitive
// to fetch a certificate from a known address using the hash as integrity
// validation.
package certs

import (
	"crypto/tls"

	"go.dedis.ch/dela/mino"
)

// Dialable is an extension of the mino.Address interface to get a network
// address that can be used to dial the distant server.
type Dialable interface {
	mino.Address

	GetDialAddress() string
}

// Storage is an interface to manage the certificates of a server.
type Storage interface {
	// Store stores the certificate with the address as the key.
	Store(mino.Address, *tls.Certificate)

	// Load returns the certificate associated with the address if any.
	Load(mino.Address) *tls.Certificate

	// Delete removes all the certificates associated with the address.
	Delete(mino.Address)

	// Range iterates over the certificates held by the store. If the callback
	// returns false, range stops the iteration.
	Range(func(addr mino.Address, cert *tls.Certificate) bool)

	// Fetch calls the address to fetch its certificate and verifies the
	// integrity with the given digest.
	Fetch(Dialable, []byte) error

	// Hash generates the digest of a certificate.
	Hash(*tls.Certificate) ([]byte, error)
}
