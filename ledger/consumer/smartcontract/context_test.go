package smartcontract

import (
	"testing"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestTransactionContext_GetTransaction(t *testing.T) {
	tx := transaction{}
	ctx := NewContext(tx, nil)

	require.Equal(t, tx, ctx.GetTransaction())
}

func TestTransactionContext_Read(t *testing.T) {
	valueAny, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)

	ctx := NewContext(
		nil,
		fakePage{
			instance: &InstanceProto{
				ContractID: "abc",
				Value:      valueAny,
			},
		},
	)

	instance, err := ctx.Read([]byte{0xab})
	require.NoError(t, err)
	require.Equal(t, "abc", instance.(ContractInstance).GetContractID())
	require.IsType(t, (*empty.Empty)(nil), instance.GetValue())

	ctx = NewContext(nil, fakePage{err: xerrors.New("oops")})
	_, err = ctx.Read(nil)
	require.EqualError(t, err, "couldn't read the entry: oops")

	ctx = NewContext(nil, fakePage{instance: &empty.Empty{}})
	_, err = ctx.Read(nil)
	require.EqualError(t, err, "couldn't decode instance: invalid instance type '*empty.Empty'")
}
