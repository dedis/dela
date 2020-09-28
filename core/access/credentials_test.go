package access

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContractCredentials_GetID(t *testing.T) {
	creds := NewContractCreds([]byte{0xaa}, "contract", "cmd")

	require.Equal(t, []byte{0xaa}, creds.GetID())
}

func TestContractCredentials_GetRule(t *testing.T) {
	creds := NewContractCreds([]byte{0xaa}, "contract", "cmd")

	require.Equal(t, "contract:cmd", creds.GetRule())
}
