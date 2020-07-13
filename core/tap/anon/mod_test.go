package anon

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
)

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

func TestTransaction_GetIdentity(t *testing.T) {
	tx, err := NewTransaction(1)
	require.NoError(t, err)
	require.Nil(t, tx.GetIdentity())
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
