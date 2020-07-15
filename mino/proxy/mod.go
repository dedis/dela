package proxy

import (
	"net/http"
)

// Proxy defines the primitives to implement an http client that handles
// client side requests
type Proxy interface {
	// Listen starts the proxy server. This call is assumed to be blocking
	Listen()

	// Stop stops the proxy server
	Stop()

	// RegisterHandler registers a new handler
	RegisterHandler(path string, handler func(http.ResponseWriter, *http.Request))
}
