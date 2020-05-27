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
	"go.dedis.ch/dela/encoding"
	"golang.org/x/xerrors"
)

// PublicKeyFactory is a public key factory for commonly known algorithms.
//
// - implements crypto.PublicKeyFactory
type PublicKeyFactory struct {
	encoder   encoding.ProtoMarshaler
	factories map[reflect.Type]crypto.PublicKeyFactory
}

// NewPublicKeyFactory returns a new instance of the common public key factory.
func NewPublicKeyFactory() PublicKeyFactory {
	factory := PublicKeyFactory{
		encoder:   encoding.NewProtoEncoder(),
		factories: make(map[reflect.Type]crypto.PublicKeyFactory),
	}

	factory.Register((*bls.PublicKeyProto)(nil), bls.NewPublicKeyFactory())

	return factory
}

// Register binds a protobuf message type to a public key factory. If a key
// already exists, it will override it.
func (f PublicKeyFactory) Register(msg proto.Message, factory crypto.PublicKeyFactory) {
	key := reflect.TypeOf(msg)
	f.factories[key] = factory
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
	factory := f.factories[key]
	if factory == nil {
		return nil, xerrors.Errorf("couldn't find factory for '%s'", key)
	}

	return factory.FromProto(in)
}

// SignatureFactory is a factory for commonly known algorithms.
type SignatureFactory struct {
	encoder   encoding.ProtoMarshaler
	factories map[reflect.Type]crypto.SignatureFactory
}

// NewSignatureFactory returns a new instance of the common signature factory.
func NewSignatureFactory() SignatureFactory {
	factory := SignatureFactory{
		encoder:   encoding.NewProtoEncoder(),
		factories: make(map[reflect.Type]crypto.SignatureFactory),
	}

	factory.Register((*bls.SignatureProto)(nil), bls.NewSignatureFactory())

	return factory
}

// Register binds a protobuf message type to a signature factory. If a key
// already exists, it will override it.
func (f SignatureFactory) Register(msg proto.Message, factory crypto.SignatureFactory) {
	key := reflect.TypeOf(msg)
	f.factories[key] = factory
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
	factory := f.factories[key]
	if factory == nil {
		return nil, xerrors.Errorf("couldn't find factory for '%s'", key)
	}

	return factory.FromProto(in)
}
