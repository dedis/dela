package smartcontract

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
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
	c.Register("bad", fakeContract{err: xerrors.New("oops")})

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

	_, err = c.Consume(SpawnTransaction{ContractID: "bad"}, fakePage{})
	require.EqualError(t, err, "couldn't execute spawn: oops")

	c.encoder = badEncoder{errMarshal: xerrors.New("oops")}
	_, err = c.Consume(spawn, fakePage{})
	require.EqualError(t, err, "couldn't marshal the value: oops")

	// 2. Consume an invoke transaction.
	c.encoder = encoding.NewProtoEncoder()
	invoke := InvokeTransaction{
		Key:      []byte{0xab},
		Argument: &empty.Empty{},
	}

	out, err = c.Consume(invoke, fakePage{instance: makeInstanceProto(t)})
	require.NoError(t, err)
	require.Equal(t, invoke.Key, out.Key)

	_, err = c.Consume(invoke, fakePage{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't read the instance: couldn't read the entry: oops")

	instancepb := makeInstanceProto(t)
	instancepb.ContractID = "unknown"
	_, err = c.Consume(invoke, fakePage{instance: instancepb})
	require.EqualError(t, err, "unknown contract with id 'unknown'")

	instancepb.ContractID = "bad"
	_, err = c.Consume(invoke, fakePage{instance: instancepb})
	require.EqualError(t, err, "couldn't invoke: oops")

	c.encoder = badEncoder{errMarshal: xerrors.New("oops")}
	_, err = c.Consume(invoke, fakePage{instance: makeInstanceProto(t)})
	require.EqualError(t, err, "couldn't marshal the value: oops")

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
	err error
}

func (c fakeContract) Spawn(ctx SpawnContext) (proto.Message, error) {
	ctx.Read([]byte{0xab})
	return &empty.Empty{}, c.err
}

func (c fakeContract) Invoke(ctx InvokeContext) (proto.Message, error) {
	ctx.Read([]byte{0xab})
	return &empty.Empty{}, c.err
}

type fakeTx struct {
	ledger.Transaction
}
