// Package certs defines a certificate store that will provide primitives to
// store and get certificates for a given address.
//
// It also provide a primitive to fetch a certificate from a known address using
// the hash as integrity validation.
//
// Documentation Last Review: 07.10.2020
//
package certs

import (
	"go.dedis.ch/dela/mino"
)

// CertChain represents a list of x509 certificates formatted as ASN.1 DER data.
// The certificates must be concatenated with no intermediate padding. Can be
// parsed with `x509.LoadCertificates`.
type CertChain []byte

// Dialable is an extension of the mino.Address interface to get a network
// address that can be used to dial the distant server.
type Dialable interface {
	mino.Address

	GetDialAddress() string
}

// Storage is an interface to manage the certificates of a server.
type Storage interface {
	// Store stores the certificate with the address as the key.
	Store(mino.Address, CertChain) error

	// Load returns the certificate associated with the address if any.
	Load(mino.Address) (CertChain, error)

	// Delete removes all the certificates associated with the address.
	Delete(mino.Address) error

	// Range iterates over the certificates held by the store. If the callback
	// returns false, range stops the iteration.
	Range(func(addr mino.Address, cert CertChain) bool) error

	// Fetch calls the address to fetch its certificate and verifies the
	// integrity with the given digest.
	Fetch(Dialable, []byte) error

	// Hash generates the digest of a certificate.
	Hash(CertChain) ([]byte, error)
}
