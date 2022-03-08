package controller

import (
	"bytes"
	"fmt"
	"net"
	nhttp "net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela"
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

func TestPromAction_Happy(t *testing.T) {
	out := new(bytes.Buffer)
	flags := make(node.FlagSet)
	path := "/fake"

	flags["path"] = path

	inj := node.NewInjector()

	proxy := fakeProxy{}
	inj.Inject(&proxy)

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
		Out:      out,
	}

	action := promAction{}
	err := action.Execute(ctx)
	require.NoError(t, err)

	require.Equal(t, fmt.Sprintf("registered prometheus service on %q", path), out.String())
	require.Len(t, proxy.handlers, 1)
	require.Equal(t, proxy.handlers[0], path)
}

func TestPromAction_ErrorInjector(t *testing.T) {
	out := new(bytes.Buffer)
	flags := make(node.FlagSet)

	inj := node.NewInjector()

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
		Out:      out,
	}

	action := promAction{}
	err := action.Execute(ctx)
	require.EqualError(t, err, "failed to resolve the proxy: couldn't find dependency for 'proxy.Proxy'")
}

func TestPromAction_ErrorCollector(t *testing.T) {
	out := new(bytes.Buffer)
	flags := make(node.FlagSet)
	path := "/fake"

	flags["path"] = path

	inj := node.NewInjector()

	proxy := fakeProxy{}
	inj.Inject(&proxy)

	ctx := node.Context{
		Injector: inj,
		Flags:    flags,
		Out:      out,
	}

	dela.PromCollectors = []prometheus.Collector{fakeCollector{}, fakeCollector{}}

	action := promAction{}
	err := action.Execute(ctx)
	require.NoError(t, err)

	require.Equal(t, "ERROR: failed to register: duplicate metrics collector "+
		"registration attemptedregistered prometheus service on \"/fake\"", out.String())
}

// -----------------------------------------------------------------------------
// Utility functions

func newFake(addr string) proxy.Proxy {
	return &fakeProxy{}
}

type fakeProxy struct {
	proxy.Proxy

	handlers []string
}

func (fakeProxy) Listen() {}

func (fakeProxy) Stop() {}

func (fakeProxy) GetAddr() net.Addr {
	return nil
}

func (f *fakeProxy) RegisterHandler(path string, handler func(nhttp.ResponseWriter, *nhttp.Request)) {
	f.handlers = append(f.handlers, path)
}

type fakeCollector struct {
	prometheus.Collector
}

func (fakeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- prometheus.NewDesc("fake", "help", nil, nil)
}
