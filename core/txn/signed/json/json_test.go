package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func TestTxFormat_Encode(t *testing.T) {
	format := txFormat{}

	ctx := fake.NewContext()

	tx := makeTx(t, 1, fake.PublicKey{}, signed.WithArg("A", []byte{1}))

	data, err := format.Encode(ctx, tx)
	require.NoError(t, err)
	require.Equal(t, `{"Nonce":1,"Args":{"A":"AQ=="},"PublicKey":{},"Signature":{}}`, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")

	_, err = format.Encode(ctx, &signed.Transaction{})
	require.EqualError(t, err, "signature is missing")

	badTx := makeTx(t, 0, fake.PublicKey{}, signed.WithSignature(fake.NewBadSignature()))
	_, err = format.Encode(ctx, badTx)
	require.EqualError(t, err, fake.Err("failed to encode signature"))

	_, err = format.Encode(fake.NewBadContext(), tx)
	require.EqualError(t, err, fake.Err("failed to marshal"))

	tx = makeTx(t, 0, badPublicKey{})
	_, err = format.Encode(fake.NewBadContextWithDelay(1), tx)
	require.EqualError(t, err, fake.Err("failed to encode public key"))
}

func TestTxFormat_Decode(t *testing.T) {
	format := txFormat{}

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, signed.PublicKeyFac{}, fake.PublicKeyFactory{})
	ctx = serde.WithFactory(ctx, signed.SignatureFac{}, fake.SignatureFactory{})

	msg, err := format.Decode(ctx, []byte(`{"Nonce":2,"Args":{"B":"AQ=="}}`))
	require.NoError(t, err)
	expected := makeTx(t, 2, fake.PublicKey{}, signed.WithArg("B", []byte{1}))
	require.Equal(t, expected, msg)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("failed to unmarshal"))

	format.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err,
		fake.Err("failed to create tx: couldn't fingerprint tx: couldn't write nonce"))

	badCtx := serde.WithFactory(ctx, signed.PublicKeyFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "public key: invalid factory '<nil>'")

	badCtx = serde.WithFactory(ctx, signed.PublicKeyFac{}, fake.NewBadPublicKeyFactory())
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, fake.Err("public key: malformed"))

	badCtx = serde.WithFactory(ctx, signed.SignatureFac{}, nil)
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "signature: invalid factory '<nil>'")

	badCtx = serde.WithFactory(ctx, signed.SignatureFac{}, fake.NewBadSignatureFactory())
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, fake.Err("signature: malformed"))
}

// -----------------------------------------------------------------------------
// Utility functions

func makeTx(t *testing.T, nonce uint64,
	pk crypto.PublicKey, opts ...signed.TransactionOption) txn.Transaction {

	opts = append([]signed.TransactionOption{signed.WithSignature(fake.Signature{})}, opts...)

	tx, err := signed.NewTransaction(nonce, pk, opts...)
	require.NoError(t, err)

	return tx
}

type badPublicKey struct {
	crypto.PublicKey
}

func (badPublicKey) Serialize(serde.Context) ([]byte, error) {
	return nil, fake.GetError()
}

func (badPublicKey) MarshalBinary() ([]byte, error) {
	return []byte{}, nil
}

func (badPublicKey) Verify([]byte, crypto.Signature) error {
	return nil
}
