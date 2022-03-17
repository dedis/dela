package controller

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/mino/proxy"
	"go.dedis.ch/dela/mino/proxy/http"
	"golang.org/x/xerrors"
)

var defaultRetry = 10
var proxyFac func(string) proxy.Proxy = http.NewHTTP

type startAction struct{}

// Execute implements node.ActionTemplate. It starts and injects the proxy http
// server.
func (a startAction) Execute(ctx node.Context) error {

	addr := ctx.Flags.String("clientaddr")

	proxyhttp := proxyFac(addr)

	ctx.Injector.Inject(proxyhttp)

	go proxyhttp.Listen()

	for i := 0; i < defaultRetry && proxyhttp.GetAddr() == nil; i++ {
		time.Sleep(time.Second)
	}

	if proxyhttp.GetAddr() == nil {
		return xerrors.Errorf("failed to start proxy server")
	}

	// We assume the listen worked proprely, however it might not be the case.
	// The log should inform the user about that.
	fmt.Fprintf(ctx.Out, "started proxy server on %s", proxyhttp.GetAddr().String())

	return nil
}

type promAction struct{}

// Execute implements node.ActionTemplate. It registers the Prometheus handler.
func (a promAction) Execute(ctx node.Context) error {
	var proxyhttp proxy.Proxy

	err := ctx.Injector.Resolve(&proxyhttp)
	if err != nil {
		return xerrors.Errorf("failed to resolve the proxy: %v", err)
	}

	path := ctx.Flags.String("path")

	for _, c := range dela.PromCollectors {
		err = prometheus.DefaultRegisterer.Register(c)
		if err != nil {
			fmt.Fprintf(ctx.Out, "ERROR: failed to register: %v", err)
		}
	}

	proxyhttp.RegisterHandler(path, promhttp.Handler().ServeHTTP)
	fmt.Fprintf(ctx.Out, "registered prometheus service on %q", path)

	return nil
}
