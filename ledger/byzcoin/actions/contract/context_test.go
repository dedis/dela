package contract

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/stretchr/testify/require"
)

func TestTransactionContext_Read(t *testing.T) {
	ctx := transactionContext{
		page: fakePage{
			store: map[string]proto.Message{
				"a": &Instance{
					ContractID: "abc",
					Value:      &any.Any{},
				},
			},
		},
	}

	instance, err := ctx.Read([]byte("a"))
	require.NoError(t, err)
	require.Equal(t, "abc", instance.GetContractID())
	require.NotNil(t, instance.Value)

	_, err = ctx.Read(nil)
	require.EqualError(t, err, "couldn't read the entry: not found")
}
