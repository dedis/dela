package actions

import (
	"reflect"

	"github.com/golang/protobuf/proto"
	any "github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/arc/darc"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"golang.org/x/xerrors"
)

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

func (f ActionFactory) Register(pb proto.Message, factory basic.ActionFactory) {
	key := reflect.TypeOf(pb)
	f.registry[key] = factory
}

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
