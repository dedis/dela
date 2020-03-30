package smartcontract

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/ledger"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&InstanceProto{},
		&SpawnTransactionProto{},
		&InvokeTransactionProto{},
		&DeleteTransactionProto{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestConsumer_Register(t *testing.T) {
	c := NewConsumer()

	c.Register("contract", fakeContract{})
	require.Len(t, c.executors, 1)

	c.Register("another contract", fakeContract{})
	require.Len(t, c.executors, 2)

	c.Register("contract", fakeContract{})
	require.Len(t, c.executors, 2)
}

func TestConsumer_GetTransactionFactory(t *testing.T) {
	c := NewConsumer()
	require.NotNil(t, c.GetTransactionFactory())
}

func TestConsumer_Consume(t *testing.T) {
	c := NewConsumer()
	c.Register("fake", fakeContract{})

	// 1. Consume a spawn transaction.
	spawn := SpawnTransaction{
		transaction: transaction{hash: []byte{0xab}},
		ContractID:  "fake",
	}

	out, err := c.Consume(spawn, fakePage{})
	require.NoError(t, err)
	require.Equal(t, spawn.hash, out.Key)

	_, err = c.Consume(SpawnTransaction{ContractID: "abc"}, fakePage{})
	require.EqualError(t, err, "unknown contract with id 'abc'")

	// 2. Consume an invoke transaction.
	invoke := InvokeTransaction{
		Key:      []byte{0xab},
		Argument: &empty.Empty{},
	}

	out, err = c.Consume(invoke, fakePage{instance: makeInstanceProto(t)})
	require.NoError(t, err)
	require.Equal(t, invoke.Key, out.Key)

	_, err = c.Consume(invoke, fakePage{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't read the instance: couldn't read the entry: oops")

	// 3. Consume a delete transaction.
	delete := DeleteTransaction{
		Key: []byte{0xab},
	}

	out, err = c.Consume(delete, fakePage{instance: makeInstanceProto(t)})
	require.NoError(t, err)
	require.True(t, out.Instance.(*InstanceProto).GetDeleted())

	_, err = c.Consume(delete, fakePage{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't read the instance: oops")

	// 4. Consume an invalid transaction.
	_, err = c.Consume(fakeTx{}, nil)
	require.EqualError(t, err, "invalid tx type 'smartcontract.fakeTx'")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeInstanceProto(t *testing.T) *InstanceProto {
	any, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)

	return &InstanceProto{
		Value:      any,
		ContractID: "fake",
		Deleted:    false,
	}
}

type fakeContract struct {
	Executor
}

func (c fakeContract) Spawn(ctx SpawnContext) (proto.Message, error) {
	ctx.Read([]byte{0xab})
	return &empty.Empty{}, nil
}

func (c fakeContract) Invoke(ctx InvokeContext) (proto.Message, error) {
	ctx.Read([]byte{0xab})
	return &empty.Empty{}, nil
}

type fakeTx struct {
	ledger.Transaction
}
