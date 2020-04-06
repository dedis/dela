package byzcoin

import (
	"bytes"
	fmt "fmt"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
)

func TestTransactionResult_GetTransactionID(t *testing.T) {
	f := func(buffer []byte) bool {
		txr := TransactionResult{txID: buffer}

		return bytes.Equal(buffer[:], txr.GetTransactionID())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestTransactionResult_String(t *testing.T) {
	f := func(buffer []byte) bool {
		txr := TransactionResult{txID: buffer}

		return txr.String() == fmt.Sprintf("TransactionResult@%#x", buffer)
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}
