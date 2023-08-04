//
// Documentation Last Review: 05.10.2020
//

package crypto

import (
	"crypto/sha256"
	"hash"

	"golang.org/x/crypto/sha3"
)

type HashAlgorithm int

const (
	Sha256 HashAlgorithm = iota
	Sha3_224
)

// hashFactory is a hash factory that is using SHA algorithms.
//
// - implements crypto.HashFactory
type hashFactory struct {
	hashType HashAlgorithm
}

// NewSha256Factory returns a new instance of the factory.
// Deprecated: use NewHashFactory instead.
func NewSha256Factory() hashFactory {
	return hashFactory{Sha256}
}

// NewHashFactory returns a new instance of the factory.
func NewHashFactory(a HashAlgorithm) hashFactory {
	return hashFactory{a}
}

// New implements crypto.HashFactory. It returns a new Hash instance.
func (f hashFactory) New() hash.Hash {
	switch f.hashType {
	case Sha256:
		return sha256.New()
	case Sha3_224:
		return sha3.New224()
	default:
		panic("unknown hash type")
	}
}
