package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cmd"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino/minogrpc"
)

func TestMinimal_Build(t *testing.T) {
	minimal := NewMinimal()

	call := &fake.Call{}
	minimal.Build(fakeCommandBuilder{call: call})

	require.Equal(t, 13, call.Len())
}

func TestMinimal_Run(t *testing.T) {
	minimal := NewMinimal()

	injector := cmd.NewInjector()

	err := minimal.Run(fakeContext{}, injector)
	require.NoError(t, err)

	var m *minogrpc.Minogrpc
	err = injector.Resolve(&m)
	require.NoError(t, err)
	require.NoError(t, m.GracefulClose())

	err = minimal.Run(fakeContext{num: 100000}, injector)
	require.EqualError(t, err, "invalid port value 100000")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeCommandBuilder struct {
	call *fake.Call
}

func (b fakeCommandBuilder) Start(...cmd.Flag) cmd.Builder {
	return b
}

func (b fakeCommandBuilder) Command(name string) cmd.CommandBuilder {
	b.call.Add(name)
	return b
}

func (b fakeCommandBuilder) Description(value string) cmd.CommandBuilder {
	b.call.Add(value)
	return b
}

func (b fakeCommandBuilder) Flags(flags ...cmd.Flag) cmd.CommandBuilder {
	b.call.Add(flags)
	return b
}

func (b fakeCommandBuilder) Action(a cmd.Action) cmd.CommandBuilder {
	b.call.Add(a)
	return b
}
