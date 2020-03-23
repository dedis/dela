package crypto

import (
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
)

func TestCryptographicRandomGenerator_Read(t *testing.T) {
	rand := CryptographicRandomGenerator{}

	f := func(buffer []byte) bool {
		n, err := rand.Read(buffer)
		require.NoError(t, err)
		require.Equal(t, len(buffer), n)

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}
