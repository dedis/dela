package anon

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func init() {
	RegisterTransactionFormat(fake.GoodFormat, fake.Format{Msg: Transaction{}})
	RegisterTransactionFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterTransactionFormat(serde.Format("BAD_TYPE"), fake.Format{Msg: fake.Message{}})
}

func TestTransaction_New(t *testing.T) {
	tx, err := NewTransaction(0)
	require.NoError(t, err)
	require.NotNil(t, tx)

	_, err = NewTransaction(0, WithHashFactory(fake.NewHashFactory(fake.NewBadHash())))
	require.EqualError(t, err, "couldn't fingerprint tx: couldn't write nonce: fake error")
}

func TestTransaction_GetID(t *testing.T) {
	tx, err := NewTransaction(0)
	require.NoError(t, err)

	id := tx.GetID()
	require.Len(t, id, 32)
}

func TestTransaction_GetNonce(t *testing.T) {
	tx, err := NewTransaction(123)
	require.NoError(t, err)

	nonce := tx.GetNonce()
	require.Equal(t, uint64(123), nonce)
}

func TestTransaction_GetIdentity(t *testing.T) {
	tx, err := NewTransaction(1)
	require.NoError(t, err)
	require.Nil(t, tx.GetIdentity())
}

func TestTransaction_GetArgs(t *testing.T) {
	tx, err := NewTransaction(5, WithArg("A", []byte{1}), WithArg("B", []byte{2}))
	require.NoError(t, err)

	args := tx.GetArgs()
	require.Contains(t, args, "A")
	require.Contains(t, args, "B")
}

func TestTransaction_GetArg(t *testing.T) {
	tx, err := NewTransaction(5, WithArg("A", []byte{1}), WithArg("B", []byte{2}))
	require.NoError(t, err)

	value := tx.GetArg("A")
	require.Equal(t, []byte{1}, value)

	value = tx.GetArg("B")
	require.Equal(t, []byte{2}, value)

	value = tx.GetArg("C")
	require.Nil(t, value)
}

func TestTransaction_Fingerprint(t *testing.T) {
	tx, err := NewTransaction(2)
	require.NoError(t, err)

	buffer := new(bytes.Buffer)
	err = tx.Fingerprint(buffer)
	require.NoError(t, err)
	require.Equal(t, "\x02\x00\x00\x00\x00\x00\x00\x00", buffer.String())

	err = tx.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write nonce: fake error")
}

func TestTransaction_Serialize(t *testing.T) {
	tx, err := NewTransaction(0)
	require.NoError(t, err)

	data, err := tx.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, `fake format`, string(data))

	_, err = tx.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "failed to encode: fake error")
}

func TestTransactionFactory_Deserialize(t *testing.T) {
	factory := NewTransactionFactory()

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.IsType(t, Transaction{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "failed to decode: fake error")

	_, err = factory.Deserialize(fake.NewContextWithFormat(serde.Format("BAD_TYPE")), nil)
	require.EqualError(t, err, "invalid transaction of type 'fake.Message'")
}
