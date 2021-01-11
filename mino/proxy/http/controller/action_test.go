package controller

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli/node"
)

func TestStartAction(t *testing.T) {
	out := new(bytes.Buffer)
	flags := make(node.FlagSet)

	flags["clientaddr"] = "127.0.0.1:3000"

	ctx := node.Context{
		Injector: node.NewInjector(),
		Flags:    flags,
		Out:      out,
	}

	action := startAction{}
	action.Execute(ctx)

	require.Equal(t, "started proxy server on 127.0.0.1:3000", out.String())
}
