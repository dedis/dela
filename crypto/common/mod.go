// Package common implements interfaces to support multiple algorithms. A public
// key factory and a signature factory are available. The supported algorithms
// are the followings:
// - BLS
package common

import (
	"reflect"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/common/json"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// PublicKeyFactory is a public key factory for commonly known algorithms.
//
// - implements crypto.PublicKeyFactory
type PublicKeyFactory struct {
	serde.UnimplementedFactory

	encoder    encoding.ProtoMarshaler
	deprecated map[reflect.Type]crypto.PublicKeyFactory
	factories  map[string]serde.Factory
}

// NewPublicKeyFactory returns a new instance of the common public key factory.
func NewPublicKeyFactory() PublicKeyFactory {
	factory := PublicKeyFactory{
		encoder:    encoding.NewProtoEncoder(),
		deprecated: make(map[reflect.Type]crypto.PublicKeyFactory),
		factories:  make(map[string]serde.Factory),
	}

	factory.Register((*bls.PublicKeyProto)(nil), bls.NewPublicKeyFactory())
	factory.RegisterAlgorithm(bls.Algorithm, bls.NewPublicKeyFactory())

	return factory
}

// RegisterAlgorithm registers the factory for the algorithm.
func (f PublicKeyFactory) RegisterAlgorithm(algo string, factory serde.Factory) {
	f.factories[algo] = factory
}

// Register binds a protobuf message type to a public key factory. If a key
// already exists, it will override it.
func (f PublicKeyFactory) Register(msg proto.Message, factory crypto.PublicKeyFactory) {
	key := reflect.TypeOf(msg)
	f.deprecated[key] = factory
}

// FromProto implements crypto.PublicKeyFactory. It returns the implementation
// of the public key if the message is a known public key message, otherwise it
// returns an error.
func (f PublicKeyFactory) FromProto(in proto.Message) (crypto.PublicKey, error) {
	inAny, ok := in.(*any.Any)
	if ok {
		var err error
		in, err = f.encoder.UnmarshalDynamicAny(inAny)
		if err != nil {
			return nil, xerrors.Errorf("couldn't decode message: %v", err)
		}
	}

	key := reflect.TypeOf(in)
	factory := f.deprecated[key]
	if factory == nil {
		return nil, xerrors.Errorf("couldn't find factory for '%s'", key)
	}

	return factory.FromProto(in)
}

// VisitJSON implements serde.Factory. It deserializes the public key for the
// given algorithm if it's known.
func (f PublicKeyFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	algo := json.Algorithm{}
	err := in.Feed(&algo)
	if err != nil {
		return nil, err
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

	encoder    encoding.ProtoMarshaler
	deprecated map[reflect.Type]crypto.SignatureFactory
	factories  map[string]serde.Factory
}

// NewSignatureFactory returns a new instance of the common signature factory.
func NewSignatureFactory() SignatureFactory {
	factory := SignatureFactory{
		encoder:    encoding.NewProtoEncoder(),
		deprecated: make(map[reflect.Type]crypto.SignatureFactory),
		factories:  make(map[string]serde.Factory),
	}

	factory.Register((*bls.SignatureProto)(nil), bls.NewSignatureFactory())
	factory.RegisterAlgorithm(bls.Algorithm, bls.NewSignatureFactory())

	return factory
}

// RegisterAlgorithm register the factory for the algorithm.
func (f SignatureFactory) RegisterAlgorithm(name string, factory serde.Factory) {
	f.factories[name] = factory
}

// Register binds a protobuf message type to a signature factory. If a key
// already exists, it will override it.
func (f SignatureFactory) Register(msg proto.Message, factory crypto.SignatureFactory) {
	key := reflect.TypeOf(msg)
	f.deprecated[key] = factory
}

// FromProto implements crypto.SignatureFactory. It returns the implementation
// of the signature if the message is a known signature message, otherwise it
// returns an error.
func (f SignatureFactory) FromProto(in proto.Message) (crypto.Signature, error) {
	inAny, ok := in.(*any.Any)
	if ok {
		var err error
		in, err = f.encoder.UnmarshalDynamicAny(inAny)
		if err != nil {
			return nil, xerrors.Errorf("couldn't decode message: %v", err)
		}
	}

	key := reflect.TypeOf(in)
	factory := f.deprecated[key]
	if factory == nil {
		return nil, xerrors.Errorf("couldn't find factory for '%s'", key)
	}

	return factory.FromProto(in)
}

// VisitJSON implements serde.Factory. It deserializes the signature using the
// factory of the algorithm if it is registered.
func (f SignatureFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	algo := json.Algorithm{}
	err := in.Feed(&algo)
	if err != nil {
		return nil, err
	}

	factory := f.factories[algo.Name]
	if factory == nil {
		return nil, xerrors.Errorf("missing factory for '%s' algorithm", algo.Name)
	}

	return factory.VisitJSON(in)
}
