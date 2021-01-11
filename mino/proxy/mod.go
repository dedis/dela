package proxy

import (
	"net"
	"net/http"
)

// Proxy defines the primitives to implement an http client that handles
// client side requests
type Proxy interface {
	// Listen starts the proxy server. This call is assumed to be blocking
	Listen()

	// Stop stops the proxy server
	Stop()

	// GetAddr returns the connection's address. This is useful in the case one
	// use the default :0 port, which makes the system use a random free port.
	GetAddr() net.Addr

	// RegisterHandler registers a new handler
	RegisterHandler(path string, handler func(http.ResponseWriter, *http.Request))
}
