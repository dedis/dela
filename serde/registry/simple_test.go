package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func TestSimpleRegistry_Register(t *testing.T) {
	registry := NewSimpleRegistry()

	registry.Register(serde.FormatJSON, fake.Format{})
	require.Len(t, registry.store, 1)

	registry.Register(serde.FormatJSON, fake.Format{})
	require.Len(t, registry.store, 1)

	registry.Register(serde.Format("A"), fake.Format{})
	require.Len(t, registry.store, 2)
}

func TestSimpleRegistry_Get(t *testing.T) {
	registry := NewSimpleRegistry()

	registry.Register(serde.FormatJSON, fake.Format{})

	format := registry.Get(serde.FormatJSON)
	require.Equal(t, fake.Format{}, format)

	format = registry.Get(serde.Format("unknown"))
	require.NotNil(t, format)

	_, err := format.Encode(serde.NewContext(nil), nil)
	require.EqualError(t, err, "format 'unknown' is not implemented")

	_, err = format.Decode(serde.NewContext(nil), nil)
	require.EqualError(t, err, "format 'unknown' is not implemented")
}
