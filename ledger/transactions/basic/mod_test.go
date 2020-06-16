package basic

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/inventory"
	types "go.dedis.ch/dela/ledger/transactions/basic/json"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
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

func TestTransaction_GetIdentity(t *testing.T) {
	tx := transaction{identity: fake.PublicKey{}}

	require.NotNil(t, tx.GetIdentity())
}

func TestTransaction_VisitJSON(t *testing.T) {
	tx := transaction{
		identity:  fake.PublicKey{},
		signature: fake.Signature{},
		task:      fakeClientTask{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(tx)
	require.NoError(t, err)
	expected := fmt.Sprintf(`{"Nonce":0,"Identity":{},"Signature":{},"Task":{"Type":"%s","Value":{}}}`,
		"go.dedis.ch/dela/ledger/transactions/basic.fakeClientTask")
	require.Equal(t, expected, string(data))

	_, err = tx.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize identity: fake error")

	_, err = tx.VisitJSON(fake.NewBadSerializerWithDelay(1))
	require.EqualError(t, err, "couldn't serialize signature: fake error")

	_, err = tx.VisitJSON(fake.NewBadSerializerWithDelay(2))
	require.EqualError(t, err, "couldn't serialize task: fake error")
}

func TestTransaction_Fingerprint(t *testing.T) {
	tx := transaction{
		nonce:    0x0102030405060708,
		identity: fake.PublicKey{},
		task:     fakeClientTask{},
	}

	buffer := new(bytes.Buffer)

	err := tx.Fingerprint(buffer)
	require.NoError(t, err)
	require.Equal(t, "\x08\x07\x06\x05\x04\x03\x02\x01\xdf\xcc", buffer.String())

	err = tx.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write nonce: fake error")

	err = tx.Fingerprint(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err, "couldn't write identity: fake error")

	tx.identity = fake.NewBadPublicKey()
	err = tx.Fingerprint(buffer)
	require.EqualError(t, err, "couldn't marshal identity: fake error")

	tx.identity = fake.PublicKey{}
	tx.task = fakeClientTask{err: xerrors.New("oops")}
	err = tx.Fingerprint(buffer)
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

func TestTransactionFactory_VisitJSON(t *testing.T) {
	factory := NewTransactionFactory(nil)
	factory.publicKeyFactory = fake.PublicKeyFactory{}
	factory.signatureFactory = fake.SignatureFactory{}
	factory.registry["fake"] = fakeTaskFactory{}

	ser := json.NewSerializer()

	var tx serverTransaction
	err := ser.Deserialize([]byte(`{"Task":{"Type":"fake"}}`), factory, &tx)
	require.NoError(t, err)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize transaction: fake error")

	_, err = factory.VisitJSON(fake.FactoryInput{Serde: fake.NewBadSerializer()})
	require.EqualError(t, err, "couldn't deserialize identity: fake error")

	_, err = factory.VisitJSON(fake.FactoryInput{Serde: fake.NewBadSerializerWithDelay(1)})
	require.EqualError(t, err, "couldn't deserialize signature: fake error")

	err = ser.Deserialize([]byte(`{"Task":{"Type":"unknown"}}`), factory, &tx)
	require.EqualError(t, xerrors.Unwrap(err), "unknown factory for type 'unknown'")

	input := fake.FactoryInput{
		Serde:   fake.NewBadSerializerWithDelay(2),
		Message: types.Transaction{Task: types.Task{Type: "fake"}},
	}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize task: fake error")

	input.Serde = fake.Serializer{}
	factory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't fingerprint: couldn't write nonce: fake error")

	factory.hashFactory = fake.NewHashFactory(&fake.Hash{})
	factory.publicKeyFactory = fake.NewPublicKeyFactory(fake.NewInvalidPublicKey())
	err = ser.Deserialize([]byte(`{"Task":{"Type":"fake"}}`), factory, &tx)
	require.EqualError(t, xerrors.Unwrap(err), "signature does not match tx: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeClientTask struct {
	serde.UnimplementedMessage

	err error
}

func (a fakeClientTask) Fingerprint(w io.Writer) error {
	w.Write([]byte{0xcc})
	return a.err
}

func (a fakeClientTask) VisitJSON(serde.Serializer) (interface{}, error) {
	return struct{}{}, nil
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
}

func (f fakeTaskFactory) VisitJSON(serde.FactoryInput) (serde.Message, error) {
	return fakeSrvTask{}, nil
}
