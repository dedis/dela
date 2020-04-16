package smartcontract

import (
	"bytes"
	"hash"
	"testing"
	"testing/quick"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/crypto/bls"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
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

func TestTransaction_Pack(t *testing.T) {
	tx := transaction{
		identity:  fakeIdentity{},
		signature: fake.Signature{},
		action:    SpawnAction{},
	}

	txpb, err := tx.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.NotNil(t, txpb.(*TransactionProto).GetSpawn())

	_, err = tx.Pack(fake.BadPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack identity: fake error")

	_, err = tx.Pack(fake.BadPackEncoder{})
	require.EqualError(t, err, "couldn't pack action: fake error")
}

func TestTransaction_ComputeHash(t *testing.T) {
	tx := transaction{
		nonce:    1,
		identity: fakeIdentity{},
		action:   SpawnAction{},
	}

	h := crypto.NewSha256Factory().New()

	hash, err := tx.computeHash(h, encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.Len(t, hash, 32)

	_, err = tx.computeHash(fake.NewBadHash(), nil)
	require.EqualError(t, err, "couldn't write nonce: fake error")

	tx.identity = fakeIdentity{err: xerrors.New("oops")}
	_, err = tx.computeHash(&fake.Hash{}, nil)
	require.EqualError(t, err, "couldn't marshal identity: oops")

	tx.identity = fakeIdentity{}
	tx.action = badAction{}
	_, err = tx.computeHash(&fake.Hash{}, nil)
	require.EqualError(t, err, "couldn't write action: oops")
}

func TestSpawnAction_Pack(t *testing.T) {
	spawn := SpawnAction{
		ContractID: "abc",
		Argument:   &wrappers.StringValue{Value: "abc"},
	}

	spawnpb, err := spawn.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*Spawn)(nil), spawnpb)

	_, err = spawn.Pack(fake.BadMarshalAnyEncoder{})
	require.EqualError(t, err, "couldn't marshal the argument: fake error")
}

func TestSpawnAction_HashTo(t *testing.T) {
	spawn := SpawnAction{}

	err := spawn.hashTo(&fake.Hash{}, encoding.NewProtoEncoder())
	require.NoError(t, err)

	spawn.Argument = &wrappers.BoolValue{Value: true}
	err = spawn.hashTo(&fake.Hash{}, encoding.NewProtoEncoder())
	require.NoError(t, err)

	err = spawn.hashTo(fake.NewBadHash(), nil)
	require.EqualError(t, err, "couldn't write contract ID: fake error")

	err = spawn.hashTo(&fake.Hash{}, fake.BadMarshalStableEncoder{})
	require.EqualError(t, err, "couldn't write argument: fake error")
}

func TestInvokeAction_Pack(t *testing.T) {
	invoke := InvokeAction{
		Key:      []byte{0xab},
		Argument: &wrappers.StringValue{Value: "abc"},
	}

	invokepb, err := invoke.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*Invoke)(nil), invokepb)

	_, err = invoke.Pack(fake.BadMarshalAnyEncoder{})
	require.EqualError(t, err, "couldn't marshal the argument: fake error")
}

func TestInvokeAction_HashTo(t *testing.T) {
	invoke := InvokeAction{}

	err := invoke.hashTo(&fake.Hash{}, encoding.NewProtoEncoder())
	require.NoError(t, err)

	invoke.Argument = &wrappers.BoolValue{Value: true}
	err = invoke.hashTo(&fake.Hash{}, encoding.NewProtoEncoder())
	require.NoError(t, err)

	err = invoke.hashTo(fake.NewBadHash(), nil)
	require.EqualError(t, err, "couldn't write key: fake error")

	err = invoke.hashTo(&fake.Hash{}, fake.BadMarshalStableEncoder{})
	require.EqualError(t, err, "couldn't write argument: fake error")
}

func TestDeleteAction_Pack(t *testing.T) {
	delete := DeleteAction{
		Key: []byte{0xab},
	}

	deletepb, err := delete.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*Delete)(nil), deletepb)
}

func TestDeleteAction_HashTo(t *testing.T) {
	delete := DeleteAction{}
	err := delete.hashTo(&fake.Hash{}, nil)
	require.NoError(t, err)

	err = delete.hashTo(fake.NewBadHash(), nil)
	require.EqualError(t, err, "couldn't write key: fake error")
}

func TestTransactionFactory_New(t *testing.T) {
	factory := NewTransactionFactory(bls.NewSigner())

	spawn, err := factory.New(SpawnAction{})
	require.NoError(t, err)
	require.IsType(t, SpawnAction{}, spawn.(transaction).action)

	invoke, err := factory.New(InvokeAction{})
	require.NoError(t, err)
	require.IsType(t, InvokeAction{}, invoke.(transaction).action)

	delete, err := factory.New(DeleteAction{})
	require.NoError(t, err)
	require.IsType(t, DeleteAction{}, delete.(transaction).action)

	factory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = factory.New(SpawnAction{})
	require.EqualError(t, err, "couldn't compute hash: couldn't write nonce: fake error")

	factory.hashFactory = fake.NewHashFactory(&fake.Hash{})
	factory.signer = fake.NewBadSigner()
	_, err = factory.New(SpawnAction{})
	require.EqualError(t, err, "couldn't sign tx: fake error")
}

func TestTransactionFactory_FromProto(t *testing.T) {
	factory := NewTransactionFactory(nil)
	factory.publicKeyFactory = fake.PublicKeyFactory{}
	factory.signatureFactory = fake.SignatureFactory{}

	tx := transaction{
		identity:  fakeIdentity{},
		signature: fake.Signature{},
	}

	// 1. Spawn transaction
	tx.action = SpawnAction{
		ContractID: "abc",
		Argument:   &wrappers.BoolValue{Value: true},
	}
	txpb, err := tx.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)

	_, err = factory.FromProto(txpb)
	require.NoError(t, err)

	factory.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = factory.FromProto(txpb)
	require.EqualError(t, err, "couldn't unmarshal argument: fake error")

	factory.encoder = encoding.NewProtoEncoder()

	// 2. Invoke transaction
	tx.action = InvokeAction{
		Key:      []byte{0xab},
		Argument: &wrappers.BoolValue{Value: true},
	}
	txpb, err = tx.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)

	_, err = factory.FromProto(txpb)
	require.NoError(t, err)

	factory.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = factory.FromProto(txpb)
	require.EqualError(t, err, "couldn't unmarshal argument: fake error")

	factory.encoder = encoding.NewProtoEncoder()

	// 3. Delete transaction
	tx.action = DeleteAction{Key: []byte{0xab}}
	txpb, err = tx.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	deleteany, err := ptypes.MarshalAny(txpb)
	require.NoError(t, err)

	_, err = factory.FromProto(txpb)
	require.NoError(t, err)

	_, err = factory.FromProto(deleteany)
	require.NoError(t, err)

	factory.encoder = fake.BadUnmarshalAnyEncoder{}
	_, err = factory.FromProto(deleteany)
	require.EqualError(t, err, "couldn't unmarshal input: fake error")

	// 4. Common
	_, err = factory.FromProto(nil)
	require.EqualError(t, err, "invalid transaction type '<nil>'")

	factory.publicKeyFactory = fake.NewBadPublicKeyFactory()
	_, err = factory.FromProto(txpb)
	require.EqualError(t, err, "couldn't decode public key: fake error")

	factory.publicKeyFactory = fake.NewPublicKeyFactory(fake.NewInvalidPublicKey())
	_, err = factory.FromProto(txpb)
	require.EqualError(t, err, "signature does not match tx: fake error")

	factory.publicKeyFactory = fake.PublicKeyFactory{}
	factory.signatureFactory = fake.NewBadSignatureFactory()
	_, err = factory.FromProto(txpb)
	require.EqualError(t, err, "couldn't decode signature: fake error")

	factory.signatureFactory = fake.SignatureFactory{}
	factory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = factory.FromProto(txpb)
	require.EqualError(t, err, "couldn't compute hash: couldn't write nonce: fake error")
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

		_, err = ci.Pack(fake.BadMarshalAnyEncoder{})
		require.EqualError(t, err, "couldn't marshal the value: fake error")

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestInstanceFactory_FromProto(t *testing.T) {
	factory := instanceFactory{encoder: encoding.NewProtoEncoder()}

	pb, err := makeInstance().Pack(factory.encoder)
	require.NoError(t, err)
	instancepb := pb.(*InstanceProto)
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

	factory.encoder = fake.BadUnmarshalAnyEncoder{}
	_, err = factory.FromProto(instanceany)
	require.EqualError(t, err, "couldn't unmarshal: fake error")

	factory.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = factory.FromProto(instancepb)
	require.EqualError(t, err, "couldn't unmarshal the value: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeIdentity struct {
	crypto.PublicKey
	err       error
	errVerify error
}

func (ident fakeIdentity) Verify([]byte, crypto.Signature) error {
	return ident.errVerify
}

func (ident fakeIdentity) MarshalBinary() ([]byte, error) {
	return []byte{0xff}, ident.err
}

func (ident fakeIdentity) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, nil
}

func (ident fakeIdentity) String() string {
	return "fakePublicKey"
}

type fakePage struct {
	inventory.Page
	instance proto.Message
	err      error
}

func (p fakePage) Read(key []byte) (proto.Message, error) {
	return p.instance, p.err
}

type badAction struct {
	action
}

func (a badAction) hashTo(hash.Hash, encoding.ProtoMarshaler) error {
	return xerrors.New("oops")
}
