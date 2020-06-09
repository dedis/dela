package basic

import (
	"bytes"
	"io"
	"testing"
	"testing/quick"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/encoding"
	internal "go.dedis.ch/dela/internal/testing"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&TransactionProto{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestTransaction_GetID(t *testing.T) {
	f := func(buffer []byte) bool {
		tx := transaction{hash: buffer}

		return bytes.Equal(buffer[:], tx.GetID())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestTransaction_GetIdentity(t *testing.T) {
	tx := transaction{identity: fake.PublicKey{}}

	require.NotNil(t, tx.GetIdentity())
}

func TestTransaction_Pack(t *testing.T) {
	tx := transaction{
		identity:  fake.PublicKey{},
		signature: fake.Signature{},
		task:      fakeClientTask{},
	}

	txpb, err := tx.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.NotNil(t, txpb.(*TransactionProto).GetTask())

	_, err = tx.Pack(fake.BadPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack identity: fake error")

	_, err = tx.Pack(fake.BadPackAnyEncoder{Counter: &fake.Counter{Value: 1}})
	require.EqualError(t, err, "couldn't pack signature: fake error")

	_, err = tx.Pack(fake.BadPackAnyEncoder{Counter: &fake.Counter{Value: 2}})
	require.EqualError(t, err, "couldn't pack task: fake error")
}

func TestTransaction_Fingerprint(t *testing.T) {
	tx := transaction{
		nonce:    0x0102030405060708,
		identity: fake.PublicKey{},
		task:     fakeClientTask{},
	}

	buffer := new(bytes.Buffer)

	err := tx.Fingerprint(buffer, nil)
	require.NoError(t, err)
	require.Equal(t, "\x08\x07\x06\x05\x04\x03\x02\x01\xdf\xcc", buffer.String())

	err = tx.Fingerprint(fake.NewBadHash(), nil)
	require.EqualError(t, err, "couldn't write nonce: fake error")

	err = tx.Fingerprint(fake.NewBadHashWithDelay(1), nil)
	require.EqualError(t, err, "couldn't write identity: fake error")

	tx.identity = fake.NewBadPublicKey()
	err = tx.Fingerprint(buffer, nil)
	require.EqualError(t, err, "couldn't marshal identity: fake error")

	tx.identity = fake.PublicKey{}
	tx.task = fakeClientTask{err: xerrors.New("oops")}
	err = tx.Fingerprint(buffer, nil)
	require.EqualError(t, err, "couldn't write task: oops")
}

func TestTransaction_String(t *testing.T) {
	tx := transaction{
		hash:     []byte{0xab},
		identity: fake.PublicKey{},
	}

	require.Equal(t, "Transaction[ab]@fake.PublicKey", tx.String())
}

func TestServerTransaction_Consume(t *testing.T) {
	tx := serverTransaction{
		transaction: transaction{task: fakeSrvTask{}},
	}

	err := tx.Consume(nil)
	require.NoError(t, err)

	tx.transaction.task = fakeClientTask{}
	err = tx.Consume(nil)
	require.EqualError(t, err, "task must implement 'basic.ServerTask'")

	tx.transaction.task = fakeSrvTask{err: xerrors.New("oops")}
	err = tx.Consume(nil)
	require.EqualError(t, err, "couldn't consume task: oops")
}

func TestTransactionFactory_New(t *testing.T) {
	factory := NewTransactionFactory(bls.NewSigner())

	clientTx, err := factory.New(fakeClientTask{})
	require.NoError(t, err)
	tx := clientTx.(transaction)
	require.NotNil(t, tx.task)
	require.NotNil(t, tx.signature)

	factory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = factory.New(fakeClientTask{})
	require.EqualError(t, err, "couldn't compute hash: couldn't write nonce: fake error")

	factory.hashFactory = fake.NewHashFactory(&fake.Hash{})
	factory.signer = fake.NewBadSigner()
	_, err = factory.New(fakeClientTask{})
	require.EqualError(t, err, "couldn't sign tx: fake error")
}

func TestTransactionFactory_FromProto(t *testing.T) {
	factory := NewTransactionFactory(nil)
	factory.publicKeyFactory = fake.PublicKeyFactory{}
	factory.signatureFactory = fake.SignatureFactory{}
	factory.Register(fakeSrvTask{}, fakeTaskFactory{})

	tx := transaction{
		identity:  fake.PublicKey{},
		signature: fake.Signature{},
		task:      fakeSrvTask{},
	}

	txpb, err := tx.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	_, err = factory.FromProto(txpb)
	require.NoError(t, err)

	txany, err := ptypes.MarshalAny(txpb)
	require.NoError(t, err)
	_, err = factory.FromProto(txany)
	require.NoError(t, err)

	_, err = factory.FromProto(nil)
	require.EqualError(t, err, "invalid transaction type '<nil>'")

	factory.encoder = fake.BadUnmarshalAnyEncoder{}
	_, err = factory.FromProto(txany)
	require.EqualError(t, err, "couldn't unmarshal input: fake error")

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

// -----------------------------------------------------------------------------
// Utility functions

type fakeClientTask struct {
	serde.UnimplementedMessage

	err error
}

func (a fakeClientTask) Fingerprint(w io.Writer, enc encoding.ProtoMarshaler) error {
	w.Write([]byte{0xcc})
	return a.err
}

func (a fakeClientTask) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, nil
}

type fakeSrvTask struct {
	fakeClientTask
	err error
}

func (a fakeSrvTask) Consume(Context, inventory.WritablePage) error {
	return a.err
}

type fakeTaskFactory struct {
	serde.UnimplementedFactory

	err error
}

func (f fakeTaskFactory) FromProto(proto.Message) (ServerTask, error) {
	return fakeSrvTask{}, f.err
}
