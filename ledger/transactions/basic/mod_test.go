package basic

import (
	"bytes"
	"io"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	RegisterTxFormat(fake.GoodFormat, fake.Format{
		Msg: ServerTransaction{ClientTransaction{identity: fake.PublicKey{}}},
	})
	RegisterTxFormat(serde.Format("BAD_IDENT"), fake.Format{
		Msg: ServerTransaction{ClientTransaction{identity: fake.NewBadPublicKey()}},
	})
	RegisterTxFormat(serde.Format("BAD_TYPE"), fake.Format{Msg: fake.Message{}})
	RegisterTxFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestClientTransaction_GetID(t *testing.T) {
	f := func(buffer []byte) bool {
		tx := ClientTransaction{hash: buffer}

		return bytes.Equal(buffer[:], tx.GetID())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestClientTransaction_GetNonce(t *testing.T) {
	f := func(nonce uint64) bool {
		tx := ClientTransaction{nonce: nonce}

		return tx.GetNonce() == nonce
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestClientTransaction_GetIdentity(t *testing.T) {
	tx := ClientTransaction{identity: fake.PublicKey{}}

	require.NotNil(t, tx.GetIdentity())
}

func TestClientTransaction_GetSignature(t *testing.T) {
	tx := ClientTransaction{signature: fake.Signature{}}

	require.Equal(t, fake.Signature{}, tx.GetSignature())
}

func TestClientTransaction_GetTask(t *testing.T) {
	tx := ClientTransaction{task: fakeSrvTask{}}

	require.Equal(t, fakeSrvTask{}, tx.GetTask())
}

func TestClientTransaction_Serialize(t *testing.T) {
	tx := ClientTransaction{}

	data, err := tx.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = tx.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode tx: fake error")
}

func TestClientTransaction_Fingerprint(t *testing.T) {
	tx := ClientTransaction{
		nonce:    0x0102030405060708,
		identity: fake.PublicKey{},
		task:     fakeClientTask{},
	}

	buffer := new(bytes.Buffer)

	err := tx.Fingerprint(buffer)
	require.NoError(t, err)
	require.Equal(t, "\x08\x07\x06\x05\x04\x03\x02\x01PK\xcc", buffer.String())

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

func TestClientTransaction_String(t *testing.T) {
	tx := ClientTransaction{
		hash:     []byte{0xab},
		identity: fake.PublicKey{},
	}

	require.Equal(t, "Transaction[ab]@fake.PublicKey", tx.String())
}

func TestServerTransactionOption_WithNonce(t *testing.T) {
	tx, err := NewServerTransaction(WithNonce(123), WithNoFingerprint())
	require.NoError(t, err)
	require.Equal(t, uint64(123), tx.nonce)
}

func TestServerTransactionOption_WithIdentity(t *testing.T) {
	tx, err := NewServerTransaction(WithIdentity(fake.PublicKey{}, fake.Signature{}), WithNoFingerprint())
	require.NoError(t, err)
	require.Equal(t, fake.PublicKey{}, tx.identity)
	require.Equal(t, fake.Signature{}, tx.signature)
}

func TestServerTransactionOption_WithTask(t *testing.T) {
	tx, err := NewServerTransaction(WithTask(fakeSrvTask{}), WithNoFingerprint())
	require.NoError(t, err)
	require.Equal(t, fakeSrvTask{}, tx.task)
}

func TestServerTransactionOption_WithHashFactory(t *testing.T) {
	tx, err := NewServerTransaction(WithHashFactory(nil))
	require.NoError(t, err)
	require.Nil(t, tx.hash)

	tx, err = NewServerTransaction(
		WithHashFactory(crypto.NewSha256Factory()),
		WithTask(fakeSrvTask{}),
		WithIdentity(fake.PublicKey{}, fake.Signature{}),
	)
	require.NoError(t, err)
	require.Len(t, tx.hash, 32)

	_, err = NewServerTransaction()
	require.EqualError(t, err, "task is nil")

	_, err = NewServerTransaction(WithTask(fakeSrvTask{}))
	require.EqualError(t, err, "identity is nil")

	_, err = NewServerTransaction(
		WithHashFactory(fake.NewHashFactory(fake.NewBadHash())),
		WithTask(fakeSrvTask{}),
		WithIdentity(fake.PublicKey{}, fake.Signature{}),
	)
	require.EqualError(t, err, "couldn't fingerprint tx: couldn't write nonce: fake error")
}

func TestServerTransaction_Consume(t *testing.T) {
	tx := ServerTransaction{
		ClientTransaction: ClientTransaction{task: fakeSrvTask{}},
	}

	err := tx.Consume(nil)
	require.NoError(t, err)

	tx.task = fakeClientTask{}
	err = tx.Consume(nil)
	require.EqualError(t, err, "task must implement 'basic.ServerTask'")

	tx.task = fakeSrvTask{err: xerrors.New("oops")}
	err = tx.Consume(nil)
	require.EqualError(t, err, "couldn't consume task 'basic.fakeSrvTask': oops")
}

func TestTransactionFactory_New(t *testing.T) {
	factory := NewTransactionFactory(bls.Generate())

	clientTx, err := factory.New(fakeClientTask{})
	require.NoError(t, err)
	tx := clientTx.(ClientTransaction)
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

func TestTransactionFactory_Register(t *testing.T) {
	factory := NewTransactionFactory(fake.NewSigner())

	factory.Register(fake.Message{}, fake.MessageFactory{})
	require.Len(t, factory.registry, 1)

	factory.Register(fake.Message{}, fake.MessageFactory{})
	require.Len(t, factory.registry, 1)
}

func TestTransactionFactory_Get(t *testing.T) {
	factory := NewTransactionFactory(fake.NewSigner())

	factory.registry["A"] = fake.MessageFactory{}
	require.NotNil(t, factory.Get("A"))
	require.Nil(t, factory.Get("B"))
}

func TestTransactionFactory_Deserialize(t *testing.T) {
	factory := NewTransactionFactory(fake.NewSigner())

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.NotNil(t, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode tx: fake error")

	_, err = factory.Deserialize(fake.NewContextWithFormat(serde.Format("BAD_TYPE")), nil)
	require.EqualError(t, err, "invalid tx type 'fake.Message'")

	_, err = factory.Deserialize(fake.NewContextWithFormat(serde.Format("BAD_IDENT")), nil)
	require.EqualError(t, err, "signature does not match tx: fake error")
}

func TestTransactionFactory_TxOf(t *testing.T) {
	factory := NewTransactionFactory(fake.NewSigner())

	tx, err := factory.TxOf(fake.NewContext(), nil)
	require.NoError(t, err)
	require.IsType(t, ServerTransaction{}, tx)

	_, err = factory.TxOf(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode tx: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeClientTask struct {
	err error
}

func (a fakeClientTask) Fingerprint(w io.Writer) error {
	w.Write([]byte{0xcc})
	return a.err
}

func (a fakeClientTask) Serialize(serde.Context) ([]byte, error) {
	return nil, nil
}

type fakeSrvTask struct {
	fakeClientTask
	err error
}

func (a fakeSrvTask) Consume(Context, inventory.WritablePage) error {
	return a.err
}
