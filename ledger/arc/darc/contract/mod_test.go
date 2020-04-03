package contract

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/arc/darc"
	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/ledger/consumer/smartcontract"
)

func TestContract_Spawn(t *testing.T) {
	contract := Contract{
		encoder: encoding.NewProtoEncoder(),
		factory: darc.Factory{},
	}

	ctx := smartcontract.SpawnContext{
		Context: fakeContext{},
		Action: smartcontract.SpawnAction{
			Argument: &darc.AccessControlProto{},
		},
	}

	arg := &darc.AccessControlProto{Rules: map[string]*darc.Expression{
		"darc:invoke": &darc.Expression{Matches: []string{"\252"}},
	}}

	pb, arcid, err := contract.Spawn(ctx)
	require.NoError(t, err)
	require.Equal(t, []byte{0xff}, arcid)
	require.True(t, proto.Equal(arg, pb), "%+v != %+v", pb, arg)
}

func TestContract_Invoke(t *testing.T) {
	contract := Contract{
		encoder: encoding.NewProtoEncoder(),
		factory: darc.Factory{},
	}

	ctx := smartcontract.InvokeContext{
		Context: fakeContext{},
		Action:  smartcontract.InvokeAction{},
	}

	pb, err := contract.Invoke(ctx)
	require.NoError(t, err)
	require.NotNil(t, pb)
}

func Test_NewGenesisTransaction(t *testing.T) {
	require.NotNil(t, NewGenesisAction())
}

func Test_NewUpdateTransaction(t *testing.T) {
	action := NewUpdateAction([]byte{0xff})
	require.Equal(t, []byte{0xff}, action.Key)
}

func Test_RegisterContract(t *testing.T) {
	c := smartcontract.NewConsumer()
	RegisterContract(c)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeIdentity struct {
	arc.Identity
}

func (ident fakeIdentity) MarshalText() ([]byte, error) {
	return []byte{0xaa}, nil
}

type fakeTransaction struct {
	consumer.Transaction
}

func (t fakeTransaction) GetID() []byte {
	return []byte{0xff}
}

func (t fakeTransaction) GetIdentity() arc.Identity {
	return fakeIdentity{}
}

type fakeInstance struct {
	consumer.Instance
}

func (i fakeInstance) GetValue() proto.Message {
	return &darc.AccessControlProto{}
}

type fakeContext struct {
	consumer.Context
}

func (ctx fakeContext) GetTransaction() consumer.Transaction {
	return fakeTransaction{}
}

func (ctx fakeContext) Read([]byte) (consumer.Instance, error) {
	return fakeInstance{}, nil
}
