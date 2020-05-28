package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReflectInjector_Resolve(t *testing.T) {
	inj := NewInjector()

	inj.Inject("abc")

	var dep string
	err := inj.Resolve(&dep)
	require.NoError(t, err)
	require.Equal(t, "abc", dep)

	var dep2 uint64
	err = inj.Resolve(&dep2)
	require.EqualError(t, err, "couldn't find dependency for 'uint64'")

	err = inj.Resolve(dep2)
	require.EqualError(t, err, "expect a pointer")
}
