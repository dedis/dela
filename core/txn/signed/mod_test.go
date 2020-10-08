package signed

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func init() {
	RegisterTransactionFormat(fake.GoodFormat, fake.Format{Msg: &Transaction{}})
	RegisterTransactionFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterTransactionFormat(serde.Format("BAD_TYPE"), fake.Format{Msg: fake.Message{}})
}

func TestTransaction_New(t *testing.T) {
	signer := bls.NewSigner()

	tx, err := NewTransaction(0, signer.GetPublicKey())
	require.NoError(t, err)
	require.NotNil(t, tx)

	require.NoError(t, tx.Sign(signer))

	tx, err = NewTransaction(0, signer.GetPublicKey(), WithSignature(tx.GetSignature()))
	require.NoError(t, err)
	require.NotNil(t, tx.GetSignature())

	_, err = NewTransaction(0, fake.PublicKey{}, WithHashFactory(fake.NewHashFactory(fake.NewBadHash())))
	require.EqualError(t, err, fake.Err("couldn't fingerprint tx: couldn't write nonce"))

	_, err = NewTransaction(1, signer.GetPublicKey(), WithSignature(tx.GetSignature()))
	require.EqualError(t, err, "invalid signature: bls verify failed: bls: invalid signature")
}

func TestTransaction_GetID(t *testing.T) {
	tx, err := NewTransaction(0, fake.PublicKey{})
	require.NoError(t, err)

	id := tx.GetID()
	require.Len(t, id, 32)
}

func TestTransaction_GetNonce(t *testing.T) {
	tx, err := NewTransaction(123, fake.PublicKey{})
	require.NoError(t, err)

	nonce := tx.GetNonce()
	require.Equal(t, uint64(123), nonce)
}

func TestTransaction_GetIdentity(t *testing.T) {
	tx, err := NewTransaction(1, fake.PublicKey{})
	require.NoError(t, err)
	require.Equal(t, fake.PublicKey{}, tx.GetIdentity())
}

func TestTransaction_GetArgs(t *testing.T) {
	tx, err := NewTransaction(5, fake.PublicKey{}, WithArg("A", []byte{1}), WithArg("B", []byte{2}))
	require.NoError(t, err)

	args := tx.GetArgs()
	require.Contains(t, args, "A")
	require.Contains(t, args, "B")
}

func TestTransaction_GetArg(t *testing.T) {
	tx, err := NewTransaction(5, fake.PublicKey{}, WithArg("A", []byte{1}), WithArg("B", []byte{2}))
	require.NoError(t, err)

	value := tx.GetArg("A")
	require.Equal(t, []byte{1}, value)

	value = tx.GetArg("B")
	require.Equal(t, []byte{2}, value)

	value = tx.GetArg("C")
	require.Nil(t, value)
}

func TestTransaction_Sign(t *testing.T) {
	signer := bls.NewSigner()

	tx, err := NewTransaction(2, signer.GetPublicKey(), WithArg("A", []byte{123}))
	require.NoError(t, err)

	err = tx.Sign(signer)
	require.NoError(t, err)
	require.NoError(t, signer.GetPublicKey().Verify(tx.hash, tx.GetSignature()))

	tx.hash = nil
	err = tx.Sign(signer)
	require.EqualError(t, err, "missing digest in transaction")

	tx.hash = []byte{1}
	err = tx.Sign(fake.Signer{})
	require.EqualError(t, err, "mismatch signer and identity")

	tx.pubkey = fake.PublicKey{}
	err = tx.Sign(fake.NewBadSigner())
	require.EqualError(t, err, fake.Err("signer"))
}

func TestTransaction_Fingerprint(t *testing.T) {
	tx, err := NewTransaction(2, fake.PublicKey{}, WithArg("A", []byte{1, 2, 3}))
	require.NoError(t, err)

	buffer := new(bytes.Buffer)
	err = tx.Fingerprint(buffer)
	require.NoError(t, err)
	require.Equal(t, "\x02\x00\x00\x00\x00\x00\x00\x00A\x01\x02\x03PK", buffer.String())

	err = tx.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, fake.Err("couldn't write nonce"))

	err = tx.Fingerprint(fake.NewBadHashWithDelay(1))
	require.EqualError(t, err, fake.Err("couldn't write arg"))

	err = tx.Fingerprint(fake.NewBadHashWithDelay(2))
	require.EqualError(t, err, fake.Err("couldn't write public key"))

	tx.pubkey = fake.NewBadPublicKey()
	err = tx.Fingerprint(buffer)
	require.EqualError(t, err, fake.Err("failed to marshal public key"))
}

func TestTransaction_Serialize(t *testing.T) {
	tx, err := NewTransaction(0, fake.PublicKey{})
	require.NoError(t, err)

	data, err := tx.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = tx.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("failed to encode"))
}

func TestTransactionFactory_Deserialize(t *testing.T) {
	factory := NewTransactionFactory()

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.IsType(t, &Transaction{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("failed to decode"))

	_, err = factory.Deserialize(fake.NewContextWithFormat(serde.Format("BAD_TYPE")), nil)
	require.EqualError(t, err, "invalid transaction of type 'fake.Message'")
}

func TestManager_Make(t *testing.T) {
	mgr := NewManager(fake.NewSigner(), nil)

	tx, err := mgr.Make(txn.Arg{Key: "a", Value: []byte{1, 2, 3}})
	require.NoError(t, err)
	require.Equal(t, uint64(0), tx.(*Transaction).nonce)
	require.Equal(t, []byte{1, 2, 3}, tx.GetArg("a"))

	mgr.hashFac = fake.NewHashFactory(fake.NewBadHash())
	_, err = mgr.Make()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create tx: ")

	mgr.hashFac = crypto.NewSha256Factory()
	mgr.signer = fake.NewBadSigner()
	_, err = mgr.Make()
	require.EqualError(t, err, fake.Err("failed to sign: signer"))
}

func TestManager_Sync(t *testing.T) {
	mgr := NewManager(fake.NewSigner(), fakeClient{})

	err := mgr.Sync()
	require.NoError(t, err)
	require.Equal(t, uint64(42), mgr.nonce)

	mgr = NewManager(fake.NewSigner(), fakeClient{err: fake.GetError()})
	err = mgr.Sync()
	require.EqualError(t, err, fake.Err("client"))
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeClient struct {
	err error
}

func (c fakeClient) GetNonce(access.Identity) (uint64, error) {
	return 42, c.err
}
