package common

import (
	"reflect"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/ledger/arc/darc"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// AccessControlFactory is an access control factory that can decode multiple
// types of access controls.
//
// - implements arc.AccessControlFactory
type AccessControlFactory struct {
	serde.UnimplementedFactory

	encoder   encoding.ProtoMarshaler
	factories map[reflect.Type]arc.AccessControlFactory
}

// NewAccessControlFactory creates a new instance of the factory with the common
// access control factories registered.
func NewAccessControlFactory() *AccessControlFactory {
	factory := &AccessControlFactory{
		encoder:   encoding.NewProtoEncoder(),
		factories: make(map[reflect.Type]arc.AccessControlFactory),
	}

	factory.Register((*darc.AccessProto)(nil), darc.Factory{})

	return factory
}

// Register binds the message to the given factory.
func (f *AccessControlFactory) Register(msg proto.Message, factory arc.AccessControlFactory) {
	key := reflect.TypeOf(msg)
	f.factories[key] = factory
}

// FromProto implements arc.AccessControlFactory. It returns the access control
// associated with the message.
func (f *AccessControlFactory) FromProto(pb proto.Message) (arc.AccessControl, error) {
	pbAny, ok := pb.(*any.Any)
	if ok {
		var err error
		pb, err = f.encoder.UnmarshalDynamicAny(pbAny)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal: %v", err)
		}
	}

	key := reflect.TypeOf(pb)
	factory := f.factories[key]
	if factory == nil {
		return nil, xerrors.Errorf("couldn't find factory for '%T'", pb)
	}

	return factory.FromProto(pb)
}
