package byzcoin

import (
	"reflect"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/consensus/viewchange"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/arc/darc"
	"go.dedis.ch/fabric/ledger/byzcoin/roster"
	"go.dedis.ch/fabric/ledger/inventory"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// taskFactory is an task factory that can process several types of tasks.
//
// - implements basic.TaskFactory
type taskFactory struct {
	encoder  encoding.ProtoMarshaler
	registry map[reflect.Type]basic.TaskFactory
}

func newtaskFactory(m mino.Mino, signer crypto.Signer,
	i inventory.Inventory) (*taskFactory, viewchange.Governance) {

	f := &taskFactory{
		encoder:  encoding.NewProtoEncoder(),
		registry: make(map[reflect.Type]basic.TaskFactory),
	}

	rosterFactory := roster.NewRosterFactory(m.GetAddressFactory(), signer.GetPublicKeyFactory())
	gov := roster.NewTaskManager(rosterFactory, i)

	f.Register((*darc.Task)(nil), darc.NewTaskFactory())
	f.Register((*roster.Task)(nil), gov)

	return f, gov
}

// Register registers the factory for the protobuf message.
func (f *taskFactory) Register(pb proto.Message, factory basic.TaskFactory) {
	key := reflect.TypeOf(pb)
	f.registry[key] = factory
}

// FromProto implements basic.TaskFactory. It returns the server task for the
// protobuf message if appropriate, otherwise an error.
func (f *taskFactory) FromProto(in proto.Message) (basic.ServerTask, error) {
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
		return nil, xerrors.Errorf("unknown task type '%T'", in)
	}

	task, err := factory.FromProto(in)
	if err != nil {
		return nil, err
	}

	return task, nil
}
