package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSha256Factory_NewDeprecated(t *testing.T) {
	factory := NewSha256Factory()
	require.NotNil(t, factory.New())
}

func TestSha256Factory_New(t *testing.T) {
	factory := NewHashFactory(Sha256)
	require.NotNil(t, factory.New())
}

func TestSha3224Factory_New(t *testing.T) {
	factory := NewHashFactory(Sha3_224)
	require.NotNil(t, factory.New())
}

func TestUnsupportedPanics(t *testing.T) {
	require.Panics(t, func() {
		factory := NewHashFactory(3)
		factory.New()
	})
}
