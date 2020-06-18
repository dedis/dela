package json

import (
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

func TestTxFormat_Encode(t *testing.T) {
	tx := makeTx(t,
		basic.WithIdentity(fake.PublicKey{}, fake.Signature{}),
		basic.WithTask(fakeServerTask{}))

	format := txFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, tx)
	require.NoError(t, err)
	expected := fmt.Sprintf(`{"Nonce":0,"Identity":{},"Signature":{},"Task":{"Type":"%s","Value":null}}`,
		basic.KeyOf(fakeServerTask{}))
	require.Equal(t, expected, string(data))

	tx = makeTx(t, basic.WithIdentity(fake.NewBadPublicKey(), nil), basic.WithNoFingerprint())
	_, err = format.Encode(ctx, tx)
	require.EqualError(t, err, "couldn't serialize identity: fake error")

	tx = makeTx(t,
		basic.WithIdentity(fake.PublicKey{}, fake.NewBadSignature()),
		basic.WithNoFingerprint())
	_, err = format.Encode(ctx, tx)
	require.EqualError(t, err, "couldn't serialize signature: fake error")

	tx = makeTx(t, basic.WithIdentity(fake.PublicKey{}, fake.Signature{}),
		basic.WithTask(fakeServerTask{err: xerrors.New("oops")}),
		basic.WithNoFingerprint())
	_, err = format.Encode(ctx, tx)
	require.EqualError(t, err, "couldn't serialize task: oops")
}

func TestTransactionFactory_VisitJSON(t *testing.T) {
	factory := basic.NewTransactionFactory(nil)
	factory.Register(fakeServerTask{}, fakeTaskFactory{})

	format := txFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})
	ctx = serdeng.WithFactory(ctx, basic.IdentityKey{}, fake.PublicKeyFactory{})
	ctx = serdeng.WithFactory(ctx, basic.SignatureKey{}, fake.SignatureFactory{})
	ctx = serdeng.WithFactory(ctx, basic.TaskKey{}, factory)

	key := basic.KeyOf(fakeServerTask{})

	tx, err := format.Decode(ctx, []byte(fmt.Sprintf(`{"Task":{"Type":"%s"}}`, key)))
	require.NoError(t, err)

	expected, err := basic.NewServerTransaction(
		basic.WithIdentity(fake.PublicKey{}, fake.Signature{}),
		basic.WithTask(fakeServerTask{}))
	require.NoError(t, err)
	require.Equal(t, expected, tx)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize transaction: fake error")

	badCtx := serdeng.WithFactory(ctx, basic.IdentityKey{}, fake.NewBadPublicKeyFactory())
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize identity: fake error")

	badCtx = serdeng.WithFactory(ctx, basic.SignatureKey{}, fake.NewBadSignatureFactory())
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize signature: fake error")

	_, err = format.Decode(ctx, []byte(`{"Task":{"Type":"unknown"}}`))
	require.EqualError(t, err, "couldn't deserialize task: factory 'unknown' not found")

	factory.Register(fakeServerTask{}, fakeTaskFactory{err: xerrors.New("oops")})
	_, err = format.Decode(ctx, []byte(fmt.Sprintf(`{"Task":{"Type":"%s"}}`, key)))
	require.EqualError(t, err, "couldn't deserialize task: oops")

	factory.Register(fakeServerTask{}, fakeTaskFactory{})
	format.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = format.Decode(ctx, []byte(fmt.Sprintf(`{"Task":{"Type":"%s"}}`, key)))
	require.EqualError(t, err, "couldn't create tx: couldn't write nonce: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeTx(t *testing.T, opts ...basic.ServerTransactionOption) basic.ServerTransaction {
	tx, err := basic.NewServerTransaction(opts...)
	require.NoError(t, err)

	return tx
}

type fakeServerTask struct {
	basic.ServerTask

	err error
}

func (t fakeServerTask) Serialize(serdeng.Context) ([]byte, error) {
	return nil, t.err
}

func (t fakeServerTask) Fingerprint(io.Writer) error {
	return nil
}

type fakeTaskFactory struct {
	err error
}

func (f fakeTaskFactory) Deserialize(serdeng.Context, []byte) (serdeng.Message, error) {
	return fakeServerTask{}, f.err
}
