package contract

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/arc/darc"
	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/ledger/consumer/smartcontract"
	"golang.org/x/xerrors"
)

func TestContract_Spawn(t *testing.T) {
	contract := Contract{
		encoder: encoding.NewProtoEncoder(),
		factory: darc.NewFactory(),
	}

	ctx := smartcontract.SpawnContext{
		Context: fakeContext{},
		Action: smartcontract.SpawnAction{
			Argument: &darc.AccessControlProto{},
		},
	}

	arg := &darc.AccessControlProto{Rules: map[string]*darc.Expression{
		"darc:invoke": {Matches: []string{"\252"}},
	}}

	pb, arcid, err := contract.Spawn(ctx)
	require.NoError(t, err)
	require.Equal(t, []byte{0xff}, arcid)
	require.True(t, proto.Equal(arg, pb), "%+v != %+v", pb, arg)

	contract.factory = badArcFactory{err: xerrors.New("oops")}
	_, _, err = contract.Spawn(ctx)
	require.EqualError(t, err, "couldn't decode argument: oops")

	contract.factory = badArcFactory{arc: fakeArc{}}
	_, _, err = contract.Spawn(ctx)
	require.EqualError(t, err,
		"'contract.fakeArc' does not implement 'darc.EvolvableAccessControl'")

	contract.factory = badArcFactory{arc: badArc{}}
	_, _, err = contract.Spawn(ctx)
	require.EqualError(t, err, "couldn't evolve darc: oops")

	contract.factory = darc.NewFactory()
	contract.encoder = fake.BadPackEncoder{}
	_, _, err = contract.Spawn(ctx)
	require.EqualError(t, err, "couldn't pack darc: fake error")
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

	ctx.Context = fakeContext{err: xerrors.New("oops")}
	_, err = contract.Invoke(ctx)
	require.EqualError(t, err, "couldn't read instance: oops")

	ctx.Context = fakeContext{}
	contract.factory = badArcFactory{err: xerrors.New("oops")}
	_, err = contract.Invoke(ctx)
	require.EqualError(t, err, "couldn't decode darc: oops")

	contract.factory = badArcFactory{arc: fakeArc{}}
	_, err = contract.Invoke(ctx)
	require.EqualError(t, err,
		"'contract.fakeArc' does not implement 'darc.EvolvableAccessControl'")

	contract.factory = darc.NewFactory()
	contract.encoder = fake.BadPackEncoder{}
	_, err = contract.Invoke(ctx)
	require.EqualError(t, err, "couldn't pack darc: fake error")
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
	err error
}

func (ctx fakeContext) GetTransaction() consumer.Transaction {
	return fakeTransaction{}
}

func (ctx fakeContext) Read([]byte) (consumer.Instance, error) {
	return fakeInstance{}, ctx.err
}

type fakeArc struct {
	arc.AccessControl
}

type badArc struct {
	arc.AccessControl
}

func (arc badArc) Evolve(string, ...arc.Identity) (darc.Access, error) {
	return darc.Access{}, xerrors.New("oops")
}

type badArcFactory struct {
	arc.AccessControlFactory
	err error
	arc arc.AccessControl
}

func (f badArcFactory) FromProto(proto.Message) (arc.AccessControl, error) {
	return f.arc, f.err
}
