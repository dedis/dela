package contract

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
)

func TestActionContext_GetArc(t *testing.T) {
	ctx := actionContext{
		arcFactory: &fakeAccessFactory{access: &fakeAccess{}},
		page: fakePage{
			store: map[string]proto.Message{"a": &empty.Empty{}},
		},
	}

	arc, err := ctx.GetArc([]byte("a"))
	require.NoError(t, err)
	require.NotNil(t, arc)

	_, err = ctx.GetArc(nil)
	require.EqualError(t, err, "couldn't read value: not found")
}

func TestActionContext_Read(t *testing.T) {
	ctx := actionContext{
		page: fakePage{
			store: map[string]proto.Message{
				"a": &Instance{
					ContractID: "abc",
					Value:      &any.Any{},
				},
				"b": &empty.Empty{},
			},
		},
	}

	instance, err := ctx.Read([]byte("a"))
	require.NoError(t, err)
	require.Equal(t, "abc", instance.GetContractID())
	require.NotNil(t, instance.Value)

	_, err = ctx.Read(nil)
	require.EqualError(t, err, "couldn't read the value: not found")

	_, err = ctx.Read([]byte("b"))
	require.EqualError(t, err,
		"invalid message type '*empty.Empty' != '*contract.Instance'")
}
