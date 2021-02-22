package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/mino/proxy"
)

type key int

const (
	requestIDKey key = 0
)

var (
	// defaultLevel can be changed to set the desired level of the logger
	defaultLevel = zerolog.ErrorLevel
)

func init() {
	setLogLevel()
}

func setLogLevel() {
	switch os.Getenv("PROXY_LOG") {
	case "warn":
		defaultLevel = zerolog.WarnLevel
	case "no":
		defaultLevel = zerolog.Disabled
	case "info":
		defaultLevel = zerolog.InfoLevel
	}
}

// NewHTTP creates a new proxy http
func NewHTTP(listenAddr string) proxy.Proxy {
	logger := dela.Logger.With().Timestamp().Str("role", "http proxy").Logger().
		Level(defaultLevel)

	nextRequestID := func() string {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}

	mux := http.NewServeMux()

	return &HTTP{
		mux: mux,
		server: &http.Server{
			Addr:    listenAddr,
			Handler: tracing(nextRequestID)(logging(logger)(mux)),
		},
		logger:     logger,
		listenAddr: listenAddr,
		quit:       make(chan struct{}),
	}
}

// HTTP defines a proxy http
//
// - implements proxy.Proxy
type HTTP struct {
	mux        *http.ServeMux
	server     *http.Server
	logger     zerolog.Logger
	listenAddr string
	quit       chan struct{}

	ln net.Listener
}

// Listen implements proxy.Proxy. This function can be called multiple times
// provided the server is not running, ie. Stop() has been called.
func (h *HTTP) Listen() {
	h.logger.Info().Msg("Client server is starting...")

	done := make(chan struct{})

	go func() {
		<-h.quit
		h.logger.Info().Msg("Server is shutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		h.server.SetKeepAlivesEnabled(false)
		err := h.server.Shutdown(ctx)
		if err != nil {
			h.logger.Fatal().Msgf("Could not gracefully shutdown the server: %v", err)
		}
		close(done)
	}()

	addr := h.listenAddr
	// if the address is empty, we use a random free port
	if addr == "" {
		addr = ":0"
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		h.logger.Panic().Msgf("failed to create conn '%s': %v", addr, err)
		return
	}

	h.ln = ln
	h.logger.Info().Msgf("Server is ready to handle requests at %s", ln.Addr())

	err = h.server.Serve(ln)
	if err != nil && err != http.ErrServerClosed {
		h.logger.Fatal().Msgf("Could not listen on %s: %v", h.listenAddr, err)
	}

	<-done
	h.logger.Info().Msg("Server stopped")
}

// Stop implements proxy.Proxy. It should be called only once in order to make a
// new Listen() successful.
func (h HTTP) Stop() {
	// we don't close it so it can be called multiple times without harm
	h.quit <- struct{}{}
}

// GetAddr implements proxy.Proxy.
func (h HTTP) GetAddr() net.Addr {
	if h.ln == nil {
		return nil
	}

	return h.ln.Addr()
}

// RegisterHandler implements proxy.Proxy
func (h HTTP) RegisterHandler(path string, handler func(http.ResponseWriter,
	*http.Request)) {

	h.mux.HandleFunc(path, handler)
}

// logging is a utility function that logs the http server events
func logging(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				requestID, ok := r.Context().Value(requestIDKey).(string)
				if !ok {
					requestID = "unknown"
				}
				logger.Info().Str("requestID", requestID).
					Str("method", r.Method).
					Str("url", r.URL.Path).
					Str("remoteAddr", r.RemoteAddr).
					Str("agent", r.UserAgent()).Msg("")
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// tracing is a utility function that adds header tracing
func tracing(nextRequestID func() string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-Id")
			if requestID == "" {
				requestID = nextRequestID()
			}
			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			w.Header().Set("X-Request-Id", requestID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
