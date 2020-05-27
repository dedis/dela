package common

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/arc"
)

func TestAccessControlFactory_Register(t *testing.T) {
	factory := NewAccessControlFactory()
	length := len(factory.factories)

	factory.Register(&empty.Empty{}, fakeFactory{})
	require.Len(t, factory.factories, length+1)

	factory.Register(&empty.Empty{}, fakeFactory{})
	require.Len(t, factory.factories, length+1)
}

func TestAccessControlFactory_FromProto(t *testing.T) {
	factory := NewAccessControlFactory()
	factory.Register(&empty.Empty{}, fakeFactory{})

	access, err := factory.FromProto(&empty.Empty{})
	require.NoError(t, err)
	require.NotNil(t, access)

	pbAny, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)
	access, err = factory.FromProto(pbAny)
	require.NoError(t, err)
	require.NotNil(t, access)

	_, err = factory.FromProto(&wrappers.BoolValue{})
	require.EqualError(t, err, "couldn't find factory for '*wrappers.BoolValue'")

	factory.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = factory.FromProto(pbAny)
	require.EqualError(t, err, "couldn't unmarshal: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeAccessControl struct {
	arc.AccessControl
}

type fakeFactory struct {
	arc.AccessControlFactory
}

func (f fakeFactory) FromProto(proto.Message) (arc.AccessControl, error) {
	return fakeAccessControl{}, nil
}
