package serde

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContext_GetFactory(t *testing.T) {
	ctx := NewContext(nil)

	ctx.factories[testKey{}] = fakeFactory{}

	factory := ctx.GetFactory(testKey{})
	require.Equal(t, fakeFactory{}, factory)

	factory = ctx.GetFactory(struct{}{})
	require.Nil(t, factory)
}

func TestContext_WithFactory(t *testing.T) {
	ctx := NewContext(nil)

	ctx2 := WithFactory(ctx, testKey{}, fakeFactory{})
	require.Len(t, ctx.factories, 0)
	require.Len(t, ctx2.factories, 1)

	ctx3 := WithFactory(ctx2, testKey{}, fakeFactory{})
	require.Len(t, ctx3.factories, 1)
}

// -----------------------------------------------------------------------------
// Utility functions

type testKey struct{}

type fakeFactory struct {
	Factory
}
