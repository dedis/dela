package controller

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/mino/proxy/http"
)

func TestStartAction(t *testing.T) {
	out := new(bytes.Buffer)
	flags := make(node.FlagSet)
	addr := "127.0.0.1:3000"

	flags["clientaddr"] = addr

	ctx := node.Context{
		Injector: node.NewInjector(),
		Flags:    flags,
		Out:      out,
	}

	action := startAction{}
	action.Execute(ctx)

	require.Equal(t, "started proxy server on "+addr, out.String())

	var proxy *http.HTTP
	err := ctx.Injector.Resolve(&proxy)
	require.NoError(t, err)

	require.Equal(t, addr, proxy.GetAddr().String())

	proxy.Stop()
}
