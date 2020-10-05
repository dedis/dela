// Package common implements the factories of the crypto primitives to allow the
// use of multiple algorithms over the same communication channel.
//
// Documentation Last Review: 05.10.2020
//
package common

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var algFormats = registry.NewSimpleRegistry()

// RegisterAlgorithmFormat registers the engine for the provided format.
func RegisterAlgorithmFormat(c serde.Format, f serde.FormatEngine) {
	algFormats.Register(c, f)
}

// Algorithm contains information about a signature algorithm.
//
// - implements serde.Message
type Algorithm struct {
	name string
}

// NewAlgorithm returns a new algorithm from the provided name.
func NewAlgorithm(name string) Algorithm {
	return Algorithm{name: name}
}

// GetName returns the name of the algorithm.
func (alg Algorithm) GetName() string {
	return alg.name
}

// Serialize implements serde.Message. It returns the serialized for the
// algorithm.
func (alg Algorithm) Serialize(ctx serde.Context) ([]byte, error) {
	format := algFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, alg)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode algorithm: %v", err)
	}

	return data, nil
}

// PublicKeyFactory is a redefinition of the crypto public key factory to
// exclude some functions that are incompatible with the logic of a common
// factory which requires specific serialization.
type PublicKeyFactory interface {
	serde.Factory

	// PublicKeyOf returns the public key associated to the data if appropriate,
	// otherwise an error.
	PublicKeyOf(serde.Context, []byte) (crypto.PublicKey, error)
}

// PublicKeyFac is a public key factory for commonly known algorithms.
//
// - implements common.PublicKeyFactory
type PublicKeyFac struct {
	factories map[string]crypto.PublicKeyFactory
}

// NewPublicKeyFactory returns a new instance of the common public key factory.
func NewPublicKeyFactory() PublicKeyFac {
	factory := PublicKeyFac{
		factories: make(map[string]crypto.PublicKeyFactory),
	}

	factory.RegisterAlgorithm(bls.Algorithm, bls.NewPublicKeyFactory())

	return factory
}

// RegisterAlgorithm registers the factory for the algorithm. It will override
// an already existing key.
func (f PublicKeyFac) RegisterAlgorithm(algo string, factory crypto.PublicKeyFactory) {
	f.factories[algo] = factory
}

// Deserialize implements serde.Factory. It looks up the format and returns the
// public key of the data if appropriate, otherwise an error.
func (f PublicKeyFac) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := algFormats.Get(ctx.GetFormat())

	m, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode algorithm: %v", err)
	}

	alg, ok := m.(Algorithm)
	if !ok {
		return nil, xerrors.Errorf("invalid message of type '%T'", m)
	}

	factory := f.factories[alg.name]
	if factory == nil {
		return nil, xerrors.Errorf("unknown algorithm '%s'", alg.name)
	}

	return factory.PublicKeyOf(ctx, data)
}

// PublicKeyOf implements crypto.PublicKeyFactory. It returns the public key of
// the data if appropriate, otherwise an error.
func (f PublicKeyFac) PublicKeyOf(ctx serde.Context, data []byte) (crypto.PublicKey, error) {
	msg, err := f.Deserialize(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg.(crypto.PublicKey), nil
}

// SignatureFactory is a factory for commonly known algorithms.
//
// - implements crypto.SignatureFactory
type SignatureFactory struct {
	factories map[string]crypto.SignatureFactory
}

// NewSignatureFactory returns a new instance of the common signature factory.
// It registers the BLS algorithm by default.
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

// Deserialize implements serde.Factory. It returns the signature associated to
// the data if appropriate, otherwise it returns an error.
func (f SignatureFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := algFormats.Get(ctx.GetFormat())

	m, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode algorithm: %v", err)
	}

	alg, ok := m.(Algorithm)
	if !ok {
		return nil, xerrors.Errorf("invalid message of type '%T'", m)
	}

	factory := f.factories[alg.name]
	if factory == nil {
		return nil, xerrors.Errorf("unknown algorithm '%s'", alg.name)
	}

	return factory.SignatureOf(ctx, data)
}

// SignatureOf implements crypto.SignatureFactory. It returns the signature
// associated to the data if appropriate, otherwise it returns an error.
func (f SignatureFactory) SignatureOf(ctx serde.Context, data []byte) (crypto.Signature, error) {
	msg, err := f.Deserialize(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg.(crypto.Signature), nil
}
