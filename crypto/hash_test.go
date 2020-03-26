package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSha256Factory_New(t *testing.T) {
	factory := NewSha256Factory()
	require.NotNil(t, factory.New())
}
