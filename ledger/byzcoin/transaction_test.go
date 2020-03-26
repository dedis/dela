package byzcoin

import (
	"bytes"
	fmt "fmt"
	"hash"
	"testing"
	"testing/quick"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/crypto"
	"golang.org/x/xerrors"
)

func TestDigest_String(t *testing.T) {
	d := Digest{0xde, 0xad, 0xbe, 0xef}
	require.Equal(t, d.String(), "0xdeadbeef000000")
}

func TestTransaction_NewTransaction(t *testing.T) {
	tx, err := newTransaction(fakeHashFactory{}, "abc")
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, "abc", tx.value)
	require.NotEmpty(t, tx.hash)

	_, err = newTransaction(fakeHashFactory{err: xerrors.New("oops")}, "abc")
	require.EqualError(t, err, "couldn't hash the tx: couldn't write t.value: oops")
}

func TestTransaction_GetID(t *testing.T) {
	f := func(buffer [32]byte) bool {
		tx := Transaction{hash: buffer}

		return bytes.Equal(buffer[:], tx.GetID())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestTransaction_Pack(t *testing.T) {
	f := func(value string) bool {
		tx := Transaction{value: value}
		packed, err := tx.Pack()
		return err == nil && packed.(*TransactionProto).Value == value
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestTransactionResult_GetTransactionID(t *testing.T) {
	f := func(buffer [32]byte) bool {
		txr := TransactionResult{txID: buffer}

		return bytes.Equal(buffer[:], txr.GetTransactionID())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestTransactionResult_String(t *testing.T) {
	f := func(buffer [32]byte) bool {
		txr := TransactionResult{txID: buffer}

		return txr.String() == fmt.Sprintf("TransactionResult@%v", Digest(buffer))
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestTransactionFactory_Create(t *testing.T) {
	factory := newTransactionFactory()

	tx, err := factory.Create("abc")
	require.NoError(t, err)
	require.Equal(t, "abc", tx.(Transaction).value)

	factory.hashFactory = fakeHashFactory{err: xerrors.New("oops")}
	_, err = factory.Create("abc")
	require.Error(t, err)
	require.Contains(t, err.Error(), "couldn't make tx: ")
}

func TestTransactionFactory_FromProto(t *testing.T) {
	factory := newTransactionFactory()

	tx, err := factory.FromProto(&TransactionProto{Value: "abc"})
	require.NoError(t, err)
	require.Equal(t, "abc", tx.(Transaction).value)

	_, err = factory.FromProto(&empty.Empty{})
	require.EqualError(t, err, "invalid message type '*empty.Empty'")

	factory.hashFactory = fakeHashFactory{err: xerrors.New("oops")}
	_, err = factory.FromProto(&TransactionProto{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "couldn't hash the tx: ")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeHash struct {
	hash.Hash
	err error
}

func (h fakeHash) Write([]byte) (int, error) {
	return 0, h.err
}

func (h fakeHash) Sum([]byte) []byte {
	return []byte{0xff}
}

type fakeHashFactory struct {
	crypto.HashFactory
	err error
}

func (f fakeHashFactory) New() hash.Hash {
	return fakeHash{err: f.err}
}
