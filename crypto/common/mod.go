// Package common implements interfaces to support multiple algorithms. A public
// key factory and a signature factory are available. The supported algorithms
// are the followings:
// - BLS
package common

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var formats = registry.NewSimpleRegistry()

func Register(c serde.Codec, f serde.Format) {
	formats.Register(c, f)
}

type Algorithm struct {
	serde.Message

	name string
}

func NewAlgorithm(name string) Algorithm {
	return Algorithm{name: name}
}

// PublicKeyFactory is a public key factory for commonly known algorithms.
//
// - implements crypto.PublicKeyFactory
// - implements serde.Factory
type PublicKeyFactory struct {
	factories map[string]crypto.PublicKeyFactory
}

// NewPublicKeyFactory returns a new instance of the common public key factory.
func NewPublicKeyFactory() PublicKeyFactory {
	factory := PublicKeyFactory{
		factories: make(map[string]crypto.PublicKeyFactory),
	}

	factory.RegisterAlgorithm(bls.Algorithm, bls.NewPublicKeyFactory())

	return factory
}

// RegisterAlgorithm registers the factory for the algorithm. It will override
// an already existing key.
func (f PublicKeyFactory) RegisterAlgorithm(algo string, factory crypto.PublicKeyFactory) {
	f.factories[algo] = factory
}

// Deserialize implements serde.Factory.
func (f PublicKeyFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := formats.Get(ctx.GetName())

	m, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	alg, ok := m.(Algorithm)
	if !ok {
		return nil, xerrors.New("invalid algorithm")
	}

	factory := f.factories[alg.name]
	if factory == nil {
		return nil, xerrors.Errorf("unknown algorithm '%s'", alg.name)
	}

	return factory.PublicKeyOf(ctx, data)
}

func (f PublicKeyFactory) PublicKeyOf(ctx serde.Context, data []byte) (crypto.PublicKey, error) {
	msg, err := f.Deserialize(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg.(crypto.PublicKey), nil
}

// SignatureFactory is a factory for commonly known algorithms.
type SignatureFactory struct {
	factories map[string]crypto.SignatureFactory
}

// NewSignatureFactory returns a new instance of the common signature factory.
func NewSignatureFactory() SignatureFactory {
	factory := SignatureFactory{
		factories: make(map[string]crypto.SignatureFactory),
	}

	factory.RegisterAlgorithm(bls.Algorithm, bls.NewSignatureFactory())

	return factory
}

// RegisterAlgorithm register the factory for the algorithm.
func (f SignatureFactory) RegisterAlgorithm(name string, factory crypto.SignatureFactory) {
	f.factories[name] = factory
}

// Deserialize implements serde.Factory.
func (f SignatureFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := formats.Get(ctx.GetName())

	m, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	alg, ok := m.(Algorithm)
	if !ok {
		return nil, xerrors.New("invalid algorithm")
	}

	factory := f.factories[alg.name]
	if factory == nil {
		return nil, xerrors.Errorf("unknown algorithm '%s'", alg.name)
	}

	return factory.SignatureOf(ctx, data)
}

func (f SignatureFactory) SignatureOf(ctx serde.Context, data []byte) (crypto.Signature, error) {
	msg, err := f.Deserialize(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg.(crypto.Signature), nil
}
