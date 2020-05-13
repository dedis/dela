package byzcoin

import (
	"reflect"
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"golang.org/x/xerrors"
)

func TestTaskFactory_Register(t *testing.T) {
	factory := &taskFactory{
		registry: make(map[reflect.Type]basic.TaskFactory),
	}

	factory.Register(&empty.Empty{}, fakeTaskFactory{})
	factory.Register(&empty.Empty{}, fakeTaskFactory{})
	factory.Register(&wrappers.BoolValue{}, fakeTaskFactory{})
	require.Len(t, factory.registry, 2)
}

func TestTaskFactory_FromProto(t *testing.T) {
	factory := &taskFactory{
		encoder:  encoding.NewProtoEncoder(),
		registry: make(map[reflect.Type]basic.TaskFactory),
	}

	factory.Register(&empty.Empty{}, fakeTaskFactory{})

	task, err := factory.FromProto(&empty.Empty{})
	require.NoError(t, err)
	require.NotNil(t, task)

	inAny, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)
	task, err = factory.FromProto(inAny)
	require.NoError(t, err)
	require.NotNil(t, task)

	factory.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = factory.FromProto(inAny)
	require.EqualError(t, err, "couldn't unmarshal message: fake error")

	_, err = factory.FromProto(&wrappers.BoolValue{})
	require.EqualError(t, err, "unknown task type '*wrappers.BoolValue'")

	factory.Register(&empty.Empty{}, fakeTaskFactory{err: xerrors.New("oops")})
	_, err = factory.FromProto(&empty.Empty{})
	require.EqualError(t, err, "couldn't decode task: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeTask struct {
	basic.ServerTask
}

type fakeTaskFactory struct {
	basic.TaskFactory
	err error
}

func (f fakeTaskFactory) FromProto(proto.Message) (basic.ServerTask, error) {
	return fakeTask{}, f.err
}
