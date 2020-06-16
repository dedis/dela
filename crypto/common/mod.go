// Package common implements interfaces to support multiple algorithms. A public
// key factory and a signature factory are available. The supported algorithms
// are the followings:
// - BLS
package common

import (
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/common/json"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// PublicKeyFactory is a public key factory for commonly known algorithms.
//
// - implements crypto.PublicKeyFactory
// - implements serde.Factory
type PublicKeyFactory struct {
	serde.UnimplementedFactory

	factories map[string]serde.Factory
}

// NewPublicKeyFactory returns a new instance of the common public key factory.
func NewPublicKeyFactory() PublicKeyFactory {
	factory := PublicKeyFactory{
		factories: make(map[string]serde.Factory),
	}

	factory.RegisterAlgorithm(bls.Algorithm, bls.NewPublicKeyFactory())

	return factory
}

// RegisterAlgorithm registers the factory for the algorithm. It will override
// an already existing key.
func (f PublicKeyFactory) RegisterAlgorithm(algo string, factory serde.Factory) {
	f.factories[algo] = factory
}

// VisitJSON implements serde.Factory. It deserializes the public key for the
// given algorithm if it's known.
func (f PublicKeyFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	algo := json.Algorithm{}
	err := in.Feed(&algo)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize algorithm: %v", err)
	}

	factory := f.factories[algo.Name]
	if factory == nil {
		return nil, xerrors.Errorf("unknown algorithm '%s'", algo.Name)
	}

	return factory.VisitJSON(in)
}

// SignatureFactory is a factory for commonly known algorithms.
type SignatureFactory struct {
	serde.UnimplementedFactory

	factories map[string]serde.Factory
}

// NewSignatureFactory returns a new instance of the common signature factory.
func NewSignatureFactory() SignatureFactory {
	factory := SignatureFactory{
		factories: make(map[string]serde.Factory),
	}

	factory.RegisterAlgorithm(bls.Algorithm, bls.NewSignatureFactory())

	return factory
}

// RegisterAlgorithm register the factory for the algorithm.
func (f SignatureFactory) RegisterAlgorithm(name string, factory serde.Factory) {
	f.factories[name] = factory
}

// VisitJSON implements serde.Factory. It deserializes the signature using the
// factory of the algorithm if it is registered.
func (f SignatureFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	algo := json.Algorithm{}
	err := in.Feed(&algo)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize algorithm: %v", err)
	}

	factory := f.factories[algo.Name]
	if factory == nil {
		return nil, xerrors.Errorf("unknown algorithm '%s'", algo.Name)
	}

	return factory.VisitJSON(in)
}
