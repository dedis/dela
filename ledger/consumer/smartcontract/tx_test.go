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
	"go.dedis.ch/fabric/crypto/bls"
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

func TestTransaction_Pack(t *testing.T) {
	tx := transaction{
		identity:  fakeIdentity{},
		signature: fakeSignature{},
		action:    SpawnAction{},
	}

	txpb, err := tx.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.NotNil(t, txpb.(*TransactionProto).GetSpawn())

	_, err = tx.Pack(badPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack identity: oops")

	_, err = tx.Pack(badPackEncoder{})
	require.EqualError(t, err, "couldn't pack action: oops")
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

	_, err = tx.computeHash(fakeHash{err: xerrors.New("oops")}, nil)
	require.EqualError(t, err, "couldn't write nonce: oops")
}

func TestSpawnAction_Pack(t *testing.T) {
	spawn := SpawnAction{
		ContractID: "abc",
		Argument:   &wrappers.StringValue{Value: "abc"},
	}

	spawnpb, err := spawn.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*Spawn)(nil), spawnpb)

	_, err = spawn.Pack(badEncoder{errMarshal: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't marshal the argument: oops")
}

func TestSpawnAction_HashTo(t *testing.T) {
	spawn := SpawnAction{}

	err := spawn.hashTo(fakeHash{}, encoding.NewProtoEncoder())
	require.NoError(t, err)

	spawn.Argument = &wrappers.BoolValue{Value: true}
	err = spawn.hashTo(fakeHash{}, encoding.NewProtoEncoder())
	require.NoError(t, err)

	err = spawn.hashTo(fakeHash{err: xerrors.New("oops")}, nil)
	require.EqualError(t, err, "couldn't write contract ID: oops")

	err = spawn.hashTo(fakeHash{}, badEncoder{errMarshal: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't write argument: oops")
}

func TestInvokeAction_Pack(t *testing.T) {
	invoke := InvokeAction{
		Key:      []byte{0xab},
		Argument: &wrappers.StringValue{Value: "abc"},
	}

	invokepb, err := invoke.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.IsType(t, (*Invoke)(nil), invokepb)

	_, err = invoke.Pack(badEncoder{errMarshal: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't marshal the argument: oops")
}

func TestInvokeAction_HashTo(t *testing.T) {
	invoke := InvokeAction{}

	err := invoke.hashTo(fakeHash{}, encoding.NewProtoEncoder())
	require.NoError(t, err)

	invoke.Argument = &wrappers.BoolValue{Value: true}
	err = invoke.hashTo(fakeHash{}, encoding.NewProtoEncoder())
	require.NoError(t, err)

	err = invoke.hashTo(fakeHash{err: xerrors.New("oops")}, nil)
	require.EqualError(t, err, "couldn't write key: oops")

	err = invoke.hashTo(fakeHash{}, badEncoder{errMarshal: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't write argument: oops")
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
	err := delete.hashTo(fakeHash{}, nil)
	require.NoError(t, err)

	err = delete.hashTo(fakeHash{err: xerrors.New("oops")}, nil)
	require.EqualError(t, err, "couldn't write key: oops")
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

	factory.hashFactory = fakeHashFactory{err: xerrors.New("oops")}
	_, err = factory.New(SpawnAction{})
	require.EqualError(t, err, "couldn't compute hash: couldn't write nonce: oops")

	factory.hashFactory = fakeHashFactory{}
	factory.signer = fakeSigner{err: xerrors.New("oops")}
	_, err = factory.New(SpawnAction{})
	require.EqualError(t, err, "couldn't sign tx: oops")
}

func TestTransactionFactory_FromProto(t *testing.T) {
	factory := NewTransactionFactory(nil)
	factory.publicKeyFactory = fakePublicKeyFactory{}
	factory.signatureFactory = fakeSignatureFactory{}

	tx := transaction{
		identity:  fakeIdentity{},
		signature: fakeSignature{},
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

	factory.encoder = badEncoder{errDynUnmarshal: xerrors.New("oops")}
	_, err = factory.FromProto(txpb)
	require.EqualError(t, err, "couldn't unmarshal argument: oops")

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

	factory.encoder = badEncoder{errDynUnmarshal: xerrors.New("oops")}
	_, err = factory.FromProto(txpb)
	require.EqualError(t, err, "couldn't unmarshal argument: oops")

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

	factory.encoder = badEncoder{errUnmarshal: xerrors.New("oops")}
	_, err = factory.FromProto(deleteany)
	require.EqualError(t, err, "couldn't unmarshal input: oops")

	// 4. Common
	_, err = factory.FromProto(nil)
	require.EqualError(t, err, "invalid transaction type '<nil>'")

	factory.publicKeyFactory = fakePublicKeyFactory{err: xerrors.New("oops")}
	_, err = factory.FromProto(txpb)
	require.EqualError(t, err, "couldn't decode public key: oops")

	factory.publicKeyFactory = fakePublicKeyFactory{errVerify: xerrors.New("oops")}
	_, err = factory.FromProto(txpb)
	require.EqualError(t, err, "signature does not match tx: oops")

	factory.publicKeyFactory = fakePublicKeyFactory{}
	factory.signatureFactory = fakeSignatureFactory{err: xerrors.New("oops")}
	_, err = factory.FromProto(txpb)
	require.EqualError(t, err, "couldn't decode signature: oops")

	factory.signatureFactory = fakeSignatureFactory{}
	factory.hashFactory = fakeHashFactory{err: xerrors.New("oops")}
	_, err = factory.FromProto(txpb)
	require.EqualError(t, err, "couldn't compute hash: couldn't write nonce: oops")
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

	factory.encoder = badEncoder{errUnmarshal: xerrors.New("oops")}
	_, err = factory.FromProto(instanceany)
	require.EqualError(t, err, "couldn't unmarshal: oops")

	factory.encoder = badEncoder{errDynUnmarshal: xerrors.New("oops")}
	_, err = factory.FromProto(instancepb)
	require.EqualError(t, err, "couldn't unmarshal the value: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeSigner struct {
	crypto.Signer
	err error
}

func (s fakeSigner) GetPublicKey() crypto.PublicKey {
	return fakeIdentity{}
}

func (s fakeSigner) Sign([]byte) (crypto.Signature, error) {
	return fakeSignature{}, s.err
}

type fakeIdentity struct {
	crypto.PublicKey
	errVerify error
}

func (ident fakeIdentity) Verify([]byte, crypto.Signature) error {
	return ident.errVerify
}

func (ident fakeIdentity) MarshalBinary() ([]byte, error) {
	return []byte{0xff}, nil
}

func (ident fakeIdentity) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, nil
}

func (ident fakeIdentity) String() string {
	return "fakePublicKey"
}

type fakeSignature struct {
	crypto.Signature
}

func (s fakeSignature) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, nil
}

type fakePublicKeyFactory struct {
	crypto.PublicKeyFactory
	err       error
	errVerify error
}

func (f fakePublicKeyFactory) FromProto(proto.Message) (crypto.PublicKey, error) {
	return fakeIdentity{errVerify: f.errVerify}, f.err
}

type fakeSignatureFactory struct {
	crypto.SignatureFactory
	err error
}

func (f fakeSignatureFactory) FromProto(proto.Message) (crypto.Signature, error) {
	return fakeSignature{}, f.err
}

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

type badPackEncoder struct {
	encoding.ProtoEncoder
}

func (e badPackEncoder) Pack(encoding.Packable) (proto.Message, error) {
	return nil, xerrors.New("oops")
}

type badPackAnyEncoder struct {
	encoding.ProtoEncoder
}

func (e badPackAnyEncoder) PackAny(encoding.Packable) (*any.Any, error) {
	return nil, xerrors.New("oops")
}

type fakePage struct {
	inventory.Page
	instance proto.Message
	err      error
}

func (p fakePage) Read(key []byte) (proto.Message, error) {
	return p.instance, p.err
}
