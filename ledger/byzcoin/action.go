package byzcoin

import (
	"reflect"

	proto "github.com/golang/protobuf/proto"
	any "github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/arc/darc"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"golang.org/x/xerrors"
)

// ActionFactory is an action factory that can process several types of actions.
//
// - implements basic.ActionFactory
type ActionFactory struct {
	encoder  encoding.ProtoMarshaler
	registry map[reflect.Type]basic.ActionFactory
}

// NewActionFactory creates a new action factory.
func NewActionFactory() basic.ActionFactory {
	f := ActionFactory{
		encoder:  encoding.NewProtoEncoder(),
		registry: make(map[reflect.Type]basic.ActionFactory),
	}

	f.Register((*darc.ActionProto)(nil), darc.NewActionFactory())

	return f
}

// Register registers the factory for the protobuf message.
func (f ActionFactory) Register(pb proto.Message, factory basic.ActionFactory) {
	key := reflect.TypeOf(pb)
	f.registry[key] = factory
}

// FromProto implements basic.ActionFactory. It returns the server action for
// the protobuf message if appropriate, otherwise an error.
func (f ActionFactory) FromProto(in proto.Message) (basic.ServerAction, error) {
	inAny, ok := in.(*any.Any)
	if ok {
		var err error
		in, err = f.encoder.UnmarshalDynamicAny(inAny)
		if err != nil {
			return nil, err
		}
	}

	key := reflect.TypeOf(in)
	factory := f.registry[key]
	if factory == nil {
		return nil, xerrors.Errorf("unknown action type '%T'", in)
	}

	action, err := factory.FromProto(in)
	if err != nil {
		return nil, err
	}

	return action, nil
}
