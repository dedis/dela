package basic

import (
	"bytes"
	"io"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestTransaction_GetID(t *testing.T) {
	f := func(buffer []byte) bool {
		tx := ClientTransaction{hash: buffer}

		return bytes.Equal(buffer[:], tx.GetID())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestTransaction_GetIdentity(t *testing.T) {
	tx := ClientTransaction{identity: fake.PublicKey{}}

	require.NotNil(t, tx.GetIdentity())
}

func TestTransaction_Fingerprint(t *testing.T) {
	tx := ClientTransaction{
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
	tx := ClientTransaction{
		hash:     []byte{0xab},
		identity: fake.PublicKey{},
	}

	require.Equal(t, "Transaction[ab]@fake.PublicKey", tx.String())
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
	factory := NewTransactionFactory(bls.NewSigner())

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
