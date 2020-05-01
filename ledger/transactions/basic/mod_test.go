package basic

import (
	"bytes"
	fmt "fmt"
	"io"
	"testing"
	"testing/quick"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/crypto/bls"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
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
		identity:  fake.PublicKey{},
		signature: fake.Signature{},
		action:    fakeClientAction{},
	}

	txpb, err := tx.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.NotNil(t, txpb.(*TransactionProto).GetAction())

	_, err = tx.Pack(fake.BadPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack identity: fake error")
}

func TestTransaction_WriteTo(t *testing.T) {
	tx := transaction{
		nonce:    1,
		identity: fake.PublicKey{},
		action:   fakeClientAction{},
	}

	w := new(bytes.Buffer)
	encoder := encoding.NewProtoEncoder()

	err := tx.Fingerprint(w, encoder)
	require.NoError(t, err)
	require.Equal(t, "0100000000000000dfcc", fmt.Sprintf("%x", w.Bytes()))

	err = tx.Fingerprint(fake.NewBadHash(), encoder)
	require.EqualError(t, err, "couldn't write nonce: fake error")

	tx.identity = fake.NewBadPublicKey()
	err = tx.Fingerprint(&fake.Hash{}, encoder)
	require.EqualError(t, err, "couldn't marshal identity: fake error")

	tx.identity = fake.PublicKey{}
	tx.action = fakeClientAction{err: xerrors.New("oops")}
	err = tx.Fingerprint(&fake.Hash{}, encoder)
	require.EqualError(t, err, "couldn't write action: oops")
}

func TestTransactionFactory_New(t *testing.T) {
	factory := NewTransactionFactory(bls.NewSigner(), nil)

	clientTx, err := factory.New(fakeClientAction{})
	require.NoError(t, err)
	tx := clientTx.(transaction)
	require.NotNil(t, tx.action)
	require.NotNil(t, tx.signature)

	factory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = factory.New(fakeClientAction{})
	require.EqualError(t, err, "couldn't compute hash: couldn't write nonce: fake error")

	factory.hashFactory = fake.NewHashFactory(&fake.Hash{})
	factory.signer = fake.NewBadSigner()
	_, err = factory.New(fakeClientAction{})
	require.EqualError(t, err, "couldn't sign tx: fake error")
}

func TestTransactionFactory_FromProto(t *testing.T) {
	factory := NewTransactionFactory(nil, fakeActionFactory{})
	factory.publicKeyFactory = fake.PublicKeyFactory{}
	factory.signatureFactory = fake.SignatureFactory{}

	tx := transaction{
		identity:  fake.PublicKey{},
		signature: fake.Signature{},
		action:    fakeSrvAction{},
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

	factory.publicKeyFactory = fake.NewBadPublicKeyFactory()
	_, err = factory.FromProto(txpb)
	require.EqualError(t, err, "couldn't decode public key: fake error")

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

type fakeClientAction struct {
	err error
}

func (a fakeClientAction) Fingerprint(w io.Writer, enc encoding.ProtoMarshaler) error {
	w.Write([]byte{0xcc})
	return a.err
}

func (a fakeClientAction) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, nil
}

type fakeSrvAction struct {
	fakeClientAction
}

func (a fakeSrvAction) Consume(Context, inventory.WritablePage) error {
	return nil
}

type fakeActionFactory struct{}

func (f fakeActionFactory) FromProto(proto.Message) (ServerAction, error) {
	return fakeSrvAction{}, nil
}
