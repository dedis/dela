package smartcontract

import (
	"bytes"
	"hash"
	"io"
	"testing"
	"testing/quick"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

func TestTransaction_GetID(t *testing.T) {
	f := func(buffer []byte) bool {
		tx := transaction{hash: buffer}

		return bytes.Equal(buffer[:], tx.GetID())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestSpawnTransaction_Pack(t *testing.T) {
	spawn := SpawnTransaction{
		ContractID: "abc",
		Argument:   &wrappers.StringValue{Value: "abc"},
	}

	spawnpb, err := spawn.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.Equal(t, spawn.ContractID, spawnpb.(*SpawnTransactionProto).GetContractID())
	require.NotNil(t, spawnpb.(*SpawnTransactionProto).GetArgument())

	_, err = spawn.Pack(badEncoder{errMarshal: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't marshal the argument: oops")
}

func TestSpawnTransaction_ComputeHash(t *testing.T) {
	f := func(id, value string) bool {
		spawn := SpawnTransaction{
			ContractID: id,
			Argument:   &wrappers.StringValue{Value: value},
		}

		var enc encoding.ProtoMarshaler = encoding.NewProtoEncoder()

		hash, err := spawn.computeHash(crypto.NewSha256Factory(), enc)
		require.NoError(t, err)
		require.Len(t, hash, 32)

		_, err = spawn.computeHash(fakeHashFactory{err: xerrors.New("oops")}, enc)
		require.EqualError(t, err, "couldn't write the contract ID: oops")

		enc = badEncoder{errMarshal: xerrors.New("oops")}
		_, err = spawn.computeHash(crypto.NewSha256Factory(), enc)
		require.EqualError(t, err, "couldn't write the argument: oops")

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestInvokeTransaction_Pack(t *testing.T) {
	invoke := InvokeTransaction{
		Key:      []byte{0xab},
		Argument: &wrappers.StringValue{Value: "abc"},
	}

	invokepb, err := invoke.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.Equal(t, invoke.Key, invokepb.(*InvokeTransactionProto).Key)
	require.NotNil(t, invokepb.(*InvokeTransactionProto).Argument)

	_, err = invoke.Pack(badEncoder{errMarshal: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't marshal the argument: oops")
}

func TestInvokeTransaction_ComputeHash(t *testing.T) {
	f := func(key []byte, value string) bool {
		invoke := InvokeTransaction{
			Key:      key,
			Argument: &wrappers.StringValue{Value: value},
		}

		var enc encoding.ProtoMarshaler = encoding.NewProtoEncoder()

		hash, err := invoke.computeHash(crypto.NewSha256Factory(), enc)
		require.NoError(t, err)
		require.Len(t, hash, 32)

		_, err = invoke.computeHash(fakeHashFactory{err: xerrors.New("oops")}, enc)
		require.EqualError(t, err, "couldn't write the key: oops")

		enc = badEncoder{errMarshal: xerrors.New("oops")}
		_, err = invoke.computeHash(crypto.NewSha256Factory(), enc)
		require.EqualError(t, err, "couldn't write the argument: oops")

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestDeleteTransaction_Pack(t *testing.T) {
	delete := DeleteTransaction{Key: []byte{0xab}}

	deletepb, err := delete.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.Equal(t, delete.Key, deletepb.(*DeleteTransactionProto).Key)
}

func TestDeleteTransaction_ComputeHash(t *testing.T) {
	f := func(key []byte) bool {
		delete := DeleteTransaction{Key: key}
		hash, err := delete.computeHash(crypto.NewSha256Factory())
		require.NoError(t, err)
		require.Len(t, hash, 32)

		_, err = delete.computeHash(fakeHashFactory{err: xerrors.New("oops")})
		require.EqualError(t, err, "couldn't write the key: oops")

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestTransactionFactory_NewSpawn(t *testing.T) {
	factory := NewTransactionFactory()

	spawn, err := factory.NewSpawn("abc", &empty.Empty{})
	require.NoError(t, err)
	require.Equal(t, spawn.ContractID, "abc")
	require.NotNil(t, spawn.Argument)

	_, err = factory.NewSpawn("abc", nil)
	require.EqualError(t, err, "argument cannot be nil")

	factory.hashFactory = fakeHashFactory{err: xerrors.New("oops")}
	_, err = factory.NewSpawn("abc", &empty.Empty{})
	require.EqualError(t, err, "couldn't hash tx: couldn't write the contract ID: oops")
}

func TestTransactionFactory_NewInvoke(t *testing.T) {
	factory := NewTransactionFactory()

	invoke, err := factory.NewInvoke([]byte{0xab}, &empty.Empty{})
	require.NoError(t, err)
	require.Equal(t, []byte{0xab}, invoke.Key)
	require.NotNil(t, invoke.Argument)

	_, err = factory.NewInvoke([]byte{0xab}, nil)
	require.EqualError(t, err, "argument cannot be nil")

	factory.hashFactory = fakeHashFactory{err: xerrors.New("oops")}
	_, err = factory.NewInvoke([]byte{0xab}, &empty.Empty{})
	require.EqualError(t, err, "couldn't hash tx: couldn't write the key: oops")
}

func TestTransactionFactory_NewDelete(t *testing.T) {
	factory := NewTransactionFactory()

	delete, err := factory.NewDelete([]byte{0xab})
	require.NoError(t, err)
	require.Equal(t, []byte{0xab}, delete.Key)

	factory.hashFactory = fakeHashFactory{err: xerrors.New("oops")}
	_, err = factory.NewDelete([]byte{0xab})
	require.EqualError(t, err, "couldn't hash tx: couldn't write the key: oops")
}

func TestTransactionFactory_FromProto(t *testing.T) {
	factory := NewTransactionFactory()

	// 1. Spawn transaction
	spawn := SpawnTransaction{
		ContractID: "abc",
		Argument:   &wrappers.BoolValue{Value: true},
	}
	spawnpb, err := spawn.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)

	tx, err := factory.FromProto(spawnpb)
	require.NoError(t, err)
	require.True(t, proto.Equal(spawn.Argument, tx.(SpawnTransaction).Argument))
	require.Equal(t, spawn.ContractID, tx.(SpawnTransaction).ContractID)

	factory.encoder = badEncoder{errDynUnmarshal: xerrors.New("oops")}
	_, err = factory.FromProto(spawnpb)
	require.EqualError(t, err, "couldn't unmarshal argument: oops")

	factory.encoder = encoding.NewProtoEncoder()

	// 2. Invoke transaction
	invoke := InvokeTransaction{
		Key:      []byte{0xab},
		Argument: &wrappers.BoolValue{Value: true},
	}
	invokepb, err := invoke.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)

	tx, err = factory.FromProto(invokepb)
	require.NoError(t, err)
	require.Equal(t, invoke.Key, tx.(InvokeTransaction).Key)
	require.True(t, proto.Equal(invoke.Argument, tx.(InvokeTransaction).Argument))

	factory.encoder = badEncoder{errDynUnmarshal: xerrors.New("oops")}
	_, err = factory.FromProto(invokepb)
	require.EqualError(t, err, "couldn't unmarshal argument: oops")

	factory.encoder = encoding.NewProtoEncoder()

	// 3. Delete transaction
	delete := DeleteTransaction{Key: []byte{0xab}}
	deletepb, err := delete.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	deleteany, err := ptypes.MarshalAny(deletepb)
	require.NoError(t, err)

	tx, err = factory.FromProto(deletepb)
	require.NoError(t, err)
	require.Equal(t, delete.Key, tx.(DeleteTransaction).Key)

	tx, err = factory.FromProto(deleteany)
	require.NoError(t, err)
	require.Equal(t, delete.Key, tx.(DeleteTransaction).Key)

	factory.encoder = badEncoder{errDynUnmarshal: xerrors.New("oops")}
	_, err = factory.FromProto(deleteany)
	require.EqualError(t, err, "couldn't unmarshal input: oops")

	// 4. Unknown
	_, err = factory.FromProto(nil)
	require.EqualError(t, err, "invalid transaction type '<nil>'")
}

func TestContractInstance_GetKey(t *testing.T) {
	f := func(key []byte) bool {
		ci := contractInstance{key: key}
		return bytes.Equal(key, ci.GetKey())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestContractInstance_GetContractID(t *testing.T) {
	f := func(id string) bool {
		ci := contractInstance{contractID: id}
		return ci.GetContractID() == id
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestContractInstance_GetValue(t *testing.T) {
	f := func(value string) bool {
		ci := contractInstance{value: &wrappers.StringValue{Value: value}}
		return proto.Equal(ci.GetValue(), &wrappers.StringValue{Value: value})
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestContractInstance_Deleted(t *testing.T) {
	ci := contractInstance{deleted: true}
	require.True(t, ci.Deleted())

	ci.deleted = false
	require.False(t, ci.Deleted())
}

func TestContractInstance_Pack(t *testing.T) {
	f := func(key []byte, id, value string, deleted bool) bool {
		ci := contractInstance{
			key:        key,
			contractID: id,
			value:      &wrappers.StringValue{Value: value},
			deleted:    deleted,
		}

		enc := encoding.NewProtoEncoder()

		cipb, err := ci.Pack(enc)
		require.NoError(t, err)
		instancepb := cipb.(*InstanceProto)
		require.Equal(t, ci.key, instancepb.GetKey())
		require.Equal(t, ci.contractID, instancepb.GetContractID())
		require.Equal(t, ci.deleted, instancepb.GetDeleted())

		msg, err := enc.UnmarshalDynamicAny(instancepb.GetValue())
		require.NoError(t, err)
		require.True(t, proto.Equal(ci.value, msg))

		_, err = ci.Pack(badEncoder{errMarshal: xerrors.New("oops")})
		require.EqualError(t, err, "couldn't marshal the value: oops")

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestInstanceFactory_FromProto(t *testing.T) {
	factory := instanceFactory{encoder: encoding.NewProtoEncoder()}

	instancepb := makeInstanceProto(t)
	instanceany, err := ptypes.MarshalAny(instancepb)
	require.NoError(t, err)

	instance, err := factory.FromProto(instancepb)
	require.NoError(t, err)
	require.IsType(t, contractInstance{}, instance)
	ci := instance.(contractInstance)
	require.Equal(t, instancepb.GetKey(), ci.key)
	require.Equal(t, instancepb.GetContractID(), ci.contractID)
	require.Equal(t, instancepb.GetDeleted(), ci.deleted)

	instance, err = factory.FromProto(instanceany)
	require.NoError(t, err)
	require.Equal(t, instancepb.GetKey(), instance.GetKey())

	factory.encoder = badEncoder{errUnmarshal: xerrors.New("oops")}
	_, err = factory.FromProto(instanceany)
	require.EqualError(t, err, "couldn't unmarshal: oops")

	factory.encoder = badEncoder{errDynUnmarshal: xerrors.New("oops")}
	_, err = factory.FromProto(instancepb)
	require.EqualError(t, err, "couldn't unmarshal the value: oops")
}

func TestTransactionContext_Read(t *testing.T) {
	valueAny, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)

	ctx := transactionContext{
		encoder: encoding.NewProtoEncoder(),
		page: fakePage{
			instance: &InstanceProto{
				ContractID: "abc",
				Value:      valueAny,
			},
		},
	}

	instance, err := ctx.Read([]byte{0xab})
	require.NoError(t, err)
	require.Equal(t, "abc", instance.GetContractID())
	require.IsType(t, (*empty.Empty)(nil), instance.GetValue())

	ctx.page = fakePage{err: xerrors.New("oops")}
	_, err = ctx.Read(nil)
	require.EqualError(t, err, "couldn't read the entry: oops")

	ctx.page = fakePage{instance: &empty.Empty{}}
	_, err = ctx.Read(nil)
	require.EqualError(t, err, "instance type '*empty.Empty' != '*smartcontract.InstanceProto'")

	ctx.page = fakePage{instance: &InstanceProto{}}
	ctx.encoder = badEncoder{errDynUnmarshal: xerrors.New("oops")}
	_, err = ctx.Read(nil)
	require.EqualError(t, err, "couldn't unmarshal the value: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeHash struct {
	hash.Hash
	err error
}

func (h fakeHash) Write([]byte) (int, error) {
	return 0, h.err
}

func (h fakeHash) Sum([]byte) []byte {
	return []byte{0xff}
}

type fakeHashFactory struct {
	crypto.HashFactory
	err error
}

func (f fakeHashFactory) New() hash.Hash {
	return fakeHash{err: f.err}
}

type badEncoder struct {
	encoding.ProtoEncoder
	errMarshal      error
	errUnmarshal    error
	errDynUnmarshal error
}

func (e badEncoder) MarshalStable(io.Writer, proto.Message) error {
	return e.errMarshal
}

func (e badEncoder) MarshalAny(proto.Message) (*any.Any, error) {
	return nil, e.errMarshal
}

func (e badEncoder) UnmarshalAny(*any.Any, proto.Message) error {
	return e.errUnmarshal
}

func (e badEncoder) UnmarshalDynamicAny(*any.Any) (proto.Message, error) {
	return nil, e.errDynUnmarshal
}

type fakePage struct {
	inventory.Page
	instance proto.Message
	err      error
}

func (p fakePage) Read(key []byte) (proto.Message, error) {
	return p.instance, p.err
}
