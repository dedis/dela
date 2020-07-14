package httpclient

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"
)

type key int

const (
	requestIDKey key = 0
)

// Httpclient defines the primitives to implement an http client that handles
// client side requests
type Httpclient interface {
	// Start starts the http server. This call is assumed to be blocking
	Start()

	// RegisterHandler registers a new handler
	RegisterHandler(path string, handler func(http.ResponseWriter, *http.Request)) error
}

// NewClient creates a new http client
func NewClient(listenAddr string) *Client {
	logger := log.New(os.Stdout, "httpclient :", log.LstdFlags)

	nextRequestID := func() string {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}

	mux := http.NewServeMux()

	return &Client{
		mux: mux,
		server: &http.Server{
			Addr:    listenAddr,
			Handler: tracing(nextRequestID)(logging(logger)(mux)),
		},
		logger:     logger,
		listenAddr: listenAddr,
	}
}

// Client defines an http client
//
// - implements Httpclient
type Client struct {
	mux        *http.ServeMux
	server     *http.Server
	logger     *log.Logger
	listenAddr string
}

// Start implements Httpclient
func (s Client) Start() {
	s.logger.Println("Client server is starting...")

	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	go func() {
		<-quit
		s.logger.Println("Server is shutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		s.server.SetKeepAlivesEnabled(false)
		err := s.server.Shutdown(ctx)
		if err != nil {
			s.logger.Fatalf("Could not gracefully shutdown the server: %v\n", err)
		}
		close(done)
	}()

	lu := &url.URL{Scheme: "http"}
	if strings.HasPrefix(s.listenAddr, ":") {
		lu.Host = "localhost" + s.listenAddr
	} else {
		lu.Host = s.listenAddr
	}

	s.logger.Println("Server is ready to handle requests at", lu)
	err := s.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		s.logger.Fatalf("Could not listen on %s: %v\n", s.listenAddr, err)
	}

	<-done
	s.logger.Println("Server stopped")
}

// RegisterHandler implements Httpclient
func (s Client) RegisterHandler(path string, handler func(http.ResponseWriter,
	*http.Request)) error {

	s.mux.HandleFunc(path, handler)
	return nil
}

// logging is a utility function that logs the http server events
func logging(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				requestID, ok := r.Context().Value(requestIDKey).(string)
				if !ok {
					requestID = "unknown"
				}
				logger.Println(requestID, r.Method, r.URL.Path, r.RemoteAddr,
					r.UserAgent())
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
