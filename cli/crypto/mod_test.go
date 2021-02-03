package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli"
)

func TestMain_Happy(t *testing.T) {
	oldPrinter := printer
	defer func() {
		printer = oldPrinter
	}()

	builder = &fakeBuilder{}
	buf := new(bytes.Buffer)
	printer = buf

	main()

	require.Empty(t, buf)
}

func TestMain_Error(t *testing.T) {
	oldPrinter := printer
	defer func() {
		printer = oldPrinter
	}()

	builder = &fakeBuilder{err: errors.New("fake")}
	buf := new(bytes.Buffer)
	printer = buf

	main()
	require.Equal(t, "fake\n", buf.String())
}

func TestRun(t *testing.T) {
	b := &fakeBuilder{}
	builder = b
	init := &fakeInit{}

	err := run([]string{"crypto"}, init)
	require.NoError(t, err)

	require.True(t, b.called)
	require.True(t, init.called)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeBuilder struct {
	cli.Builder
	err    error
	called bool
}

func (f *fakeBuilder) Build() cli.Application {
	f.called = true
	return &fakeApp{err: f.err}
}

func (f *fakeBuilder) SetCommand(name string) cli.CommandBuilder {
	return fakeCommandBuilder{}
}

type fakeCommandBuilder struct {
}

func (b fakeCommandBuilder) SetSubCommand(name string) cli.CommandBuilder {
	return b
}

func (b fakeCommandBuilder) SetDescription(value string) {
}

func (b fakeCommandBuilder) SetFlags(flags ...cli.Flag) {
}

func (b fakeCommandBuilder) SetAction(a cli.Action) {
}

type fakeApp struct {
	err error
}

func (f fakeApp) Run(arguments []string) error {
	return f.err
}

type fakeInit struct {
	called bool
}

func (f *fakeInit) SetCommands(cli.Provider) {
	f.called = true
}
