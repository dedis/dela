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

	require.Equal(t, 12, call.Len())
}

func TestMinimal_Run(t *testing.T) {
	minimal := NewMinimal()

	injector := cmd.NewInjector()

	err := minimal.Run(injector)
	require.NoError(t, err)

	var m *minogrpc.Minogrpc
	err = injector.Resolve(&m)
	require.NoError(t, err)

	require.NoError(t, m.GracefulClose())
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeCommandBuilder struct {
	call *fake.Call
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
