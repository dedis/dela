package controller

import (
	"bytes"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/mino/proxy"
	"go.dedis.ch/dela/mino/proxy/http"
)

func TestStartAction_Happy(t *testing.T) {
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
	err := action.Execute(ctx)
	require.NoError(t, err)

	require.Equal(t, "started proxy server on "+addr, out.String())

	var proxy *http.HTTP
	err = ctx.Injector.Resolve(&proxy)
	require.NoError(t, err)

	require.Equal(t, addr, proxy.GetAddr().String())

	proxy.Stop()
}

func TestStartAction_Error(t *testing.T) {
	defaultRetry = 1

	oldFac := proxyFac
	defer func() {
		proxyFac = oldFac
	}()

	proxyFac = newFake

	out := new(bytes.Buffer)
	flags := make(node.FlagSet)
	addr := "fake"

	flags["clientaddr"] = addr

	ctx := node.Context{
		Injector: node.NewInjector(),
		Flags:    flags,
		Out:      out,
	}

	action := startAction{}
	err := action.Execute(ctx)
	require.Error(t, err, "failed to start proxy server")
}

// -----------------------------------------------------------------------------
// Utility functions

func newFake(addr string) proxy.Proxy {
	return fakeProxy{}
}

type fakeProxy struct {
	proxy.Proxy
}

func (fakeProxy) Listen() {}

func (fakeProxy) Stop() {}

func (fakeProxy) GetAddr() net.Addr {
	return nil
}
