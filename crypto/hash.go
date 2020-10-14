//
// Documentation Last Review: 05.10.2020
//

package crypto

import (
	"crypto/sha256"
	"hash"
)

// Sha256Factory is a hash factory that is using SHA256.
//
// - implements crypto.HashFactory
type Sha256Factory struct{}

// NewSha256Factory returns a new instance of the factory.
func NewSha256Factory() Sha256Factory {
	return Sha256Factory{}
}

// New implements crypto.HashFactory. It returns a new SHA256 instance.
func (f Sha256Factory) New() hash.Hash {
	return sha256.New()
}
