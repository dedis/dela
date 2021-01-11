package controller

import (
	"fmt"
	"time"

	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/mino/proxy/http"
)

type startAction struct{}

// Execute implements node.ActionTemplate. It starts and injects the proxy http
// server.
func (a startAction) Execute(ctx node.Context) error {
	addr := ctx.Flags.String("clientaddr")

	proxyhttp := http.NewHTTP(addr)

	ctx.Injector.Inject(proxyhttp)

	go proxyhttp.Listen()
	time.Sleep(time.Second)

	fmt.Fprintf(ctx.Out, "started proxy server on %s", proxyhttp.GetAddr().String())

	return nil
}
