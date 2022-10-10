// Package minogrpc implements a network overlay using gRPC.
//
// Documentation Last Review: 07.10.2020
//

package minogrpc

import (
	"context"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	otgrpc "github.com/opentracing-contrib/go-grpc"
	opentracing "github.com/opentracing/opentracing-go"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/internal/tracing"
	"go.dedis.ch/dela/internal/traffic"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

var (
	segmentMatch = regexp.MustCompile("^[a-zA-Z0-9]+$")
	addressFac   = session.AddressFactory{}
)

// NewAddressFactory returns a new address factory.
func NewAddressFactory() mino.AddressFactory {
	return addressFac
}

// ParseAddress is a helper to create a TCP network address.
func ParseAddress(ip string, port uint16) net.Addr {
	return &net.TCPAddr{
		IP:   net.ParseIP(ip),
		Port: int(port),
	}
}

// listener is the default listener used to create the socket. Having it as a
// variable is convenient for the tests.
var listener = net.Listen

// Joinable is an extension of the mino.Mino interface to allow distant servers
// to join a network of participants.
type Joinable interface {
	mino.Mino

	// GetCertificateChain returns the certificate chain of the instance.
	GetCertificateChain() certs.CertChain

	// GetCertificateStore returns the certificate storage which contains every
	// known peer certificate.
	GetCertificateStore() certs.Storage

	// GenerateToken returns a token that can be provided by a distant peer to
	// mutually share certificates with this instance.
	GenerateToken(expiration time.Duration) string

	// Join tries to mutually share certificates of the distant address in
	// parameter using the token as a credential. The certificate of the distant
	// address digest is compared against the one in parameter.
	//
	// The token and the certificate digest are provided by the distant peer
	// over a secure channel.
	//
	// Only the "host" and "path" parts are used in the URL, which must be of
	// form //<host>:<port>/<path>
	Join(addr *url.URL, token string, certHash []byte) error
}

// Endpoint defines the requirement of an endpoint. Since the endpoint can be
// called multiple times concurrently we need a mutex and we need to use the
// same sender/receiver.
type Endpoint struct {
	// We need this mutex to prevent two processes from concurrently checking
	// that the stream session must be created. Using a sync.Map would require
	// to use the "LoadOrStore" function, which would make us create the session
	// each time, but only saving it the first time.
	sync.RWMutex

	Handler mino.Handler
	Factory serde.Factory
	streams map[string]session.Session
}

// Minogrpc is an implementation of a minimalist network overlay using gRPC
// internally to communicate with distant peers.
//
// - implements mino.Mino
// - implements fmt.Stringer
type Minogrpc struct {
	*overlay

	server    *grpc.Server
	segments  []string
	endpoints map[string]*Endpoint
	started   chan struct{}
	closing   chan error
}

type minoTemplate struct {
	myAddr session.Address
	router router.Router
	fac    mino.AddressFactory
	certs  certs.Storage
	secret interface{}
	public interface{}
	curve  elliptic.Curve
	random io.Reader
	cert   *tls.Certificate
	useTLS bool
}

// Option is the type to set some fields when instantiating an overlay.
type Option func(*minoTemplate)

// WithStorage is an option to set a different certificate storage.
func WithStorage(certs certs.Storage) Option {
	return func(tmpl *minoTemplate) {
		tmpl.certs = certs
	}
}

// WithCertificateKey is an option to set the key of the server certificate.
func WithCertificateKey(secret, public interface{}) Option {
	return func(tmpl *minoTemplate) {
		tmpl.secret = secret
		tmpl.public = public
	}
}

// WithRandom is an option to set the randomness if the certificate private key
// needs to be generated.
func WithRandom(r io.Reader) Option {
	return func(tmpl *minoTemplate) {
		tmpl.random = r
	}
}

// WithCert is an option to set the node's certificate in case it is not already
// present in the certificate store.
func WithCert(cert *tls.Certificate) Option {
	return func(tmpl *minoTemplate) {
		tmpl.cert = cert
	}
}

// DisableTLS disables TLS encryption on gRPC connections. It takes precedence
// over WithCert.
func DisableTLS() Option {
	return func(tmpl *minoTemplate) {
		tmpl.useTLS = false
	}
}

// NewMinogrpc creates and starts a new instance. it will try to listen for the
// address and returns an error if it fails. "listen" is the local address,
// while "public" is the public node address. If public is empty it uses the
// local address. Public does not support any scheme, it should be of form
// //<hostname>:<port>/<path>.
func NewMinogrpc(listen net.Addr, public *url.URL, router router.Router, opts ...Option) (*Minogrpc, error) {
	socket, err := listener(listen.Network(), listen.String())
	if err != nil {
		return nil, xerrors.Errorf("failed to bind: %v", err)
	}

	if public == nil {
		public, err = url.Parse("//" + socket.Addr().String())
		if err != nil {
			return nil, xerrors.Errorf("failed to parse public URL: %v", err)
		}
	}

	dela.Logger.Info().Msgf("public URL is: %s", public.String())

	tmpl := minoTemplate{
		myAddr: session.NewAddress(public.Host + public.Path),
		router: router,
		fac:    addressFac,
		certs:  certs.NewInMemoryStore(),
		curve:  elliptic.P521(),
		random: rand.Reader,
		useTLS: true,
	}

	for _, opt := range opts {
		opt(&tmpl)
	}

	o, err := newOverlay(&tmpl)
	if err != nil {
		socket.Close()
		return nil, xerrors.Errorf("overlay: %v", err)
	}

	dialAddr := o.myAddr.GetDialAddress()
	tracer, err := getTracerForAddr(dialAddr)
	if err != nil {
		return nil, xerrors.Errorf("failed to get tracer for addr %s: %v", dialAddr, err)
	}

	srvOpts := []grpc.ServerOption{
		grpc.UnaryInterceptor(otgrpc.OpenTracingServerInterceptor(tracer, otgrpc.SpanDecorator(decorateServerTrace))),
		grpc.StreamInterceptor(otgrpc.OpenTracingStreamServerInterceptor(tracer, otgrpc.SpanDecorator(decorateServerTrace))),
	}

	if !tmpl.useTLS {
		dela.Logger.Warn().Msg("⚠️ running in insecure mode, you should not " +
			"publicly expose the node's socket")
	}

	if tmpl.useTLS {
		chainBuf := o.GetCertificateChain()
		certs, err := x509.ParseCertificates(chainBuf)
		if err != nil {
			socket.Close()
			return nil, xerrors.Errorf("failed to parse chain: %v", err)
		}

		certsBuf := make([][]byte, len(certs))
		for i, c := range certs {
			certsBuf[i] = c.Raw
		}

		creds := credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{{
				Certificate: certsBuf,
				Leaf:        certs[0],
				PrivateKey:  o.secret,
			}},
			MinVersion: tls.VersionTLS12,
		})

		srvOpts = append(srvOpts, grpc.Creds(creds))
	}

	server := grpc.NewServer(srvOpts...)

	m := &Minogrpc{
		overlay:   o,
		server:    server,
		segments:  nil,
		endpoints: make(map[string]*Endpoint),
		started:   make(chan struct{}),
		closing:   make(chan error, 1),
	}

	// Counter needs to be >=1 for asynchronous call to Add.
	m.closer.Add(1)

	ptypes.RegisterOverlayServer(server, &overlayServer{
		overlay:   o,
		endpoints: m.endpoints,
	})

	dela.Logger.Info().Msgf("listening on: %s", socket.Addr().String())

	m.listen(socket)

	return m, nil
}

// GetAddressFactory implements mino.Mino. It returns the address
// factory.
func (m *Minogrpc) GetAddressFactory() mino.AddressFactory {
	return NewAddressFactory()
}

// GetAddress implements mino.Mino. It returns the address of the server.
func (m *Minogrpc) GetAddress() mino.Address {
	return m.overlay.myAddr
}

// GenerateToken implements minogrpc.Joinable. It generates and returns a new
// token that will be valid for the given amount of time.
func (m *Minogrpc) GenerateToken(expiration time.Duration) string {
	return m.tokens.Generate(expiration)
}

// GracefulStop first stops the grpc server then waits for the remaining
// handlers to close.
func (m *Minogrpc) GracefulStop() error {
	m.server.GracefulStop()

	return m.postCheckClose()
}

// Stop stops the server immediately.
func (m *Minogrpc) Stop() error {
	m.server.Stop()

	return m.postCheckClose()
}

func (m *Minogrpc) postCheckClose() error {
	m.closer.Wait()

	err := <-m.closing
	if err != nil {
		return xerrors.Errorf("server stopped unexpectedly: %v", err)
	}

	numConns := m.overlay.connMgr.Len()
	if numConns > 0 {
		return xerrors.Errorf("connection manager not empty: %d", numConns)
	}

	err = tracing.CloseAll()
	if err != nil {
		return xerrors.Errorf("failed to close tracers: %v", err)
	}

	return nil
}

// WithSegment returns a new mino instance that will have its URI path extended
// with the provided segment. The segment can not be empty and should match
// [a-zA-Z0-9]+
func (m *Minogrpc) WithSegment(segment string) mino.Mino {

	if segment == "" {
		return m
	}

	newM := &Minogrpc{
		server:    m.server,
		overlay:   m.overlay,
		segments:  append(m.segments, segment),
		endpoints: m.endpoints,
	}

	return newM
}

// CreateRPC implements Mino. It returns a newly created rpc with the provided
// name and reserved for the current namespace. When contacting distant peers,
// it will only talk to mirrored RPCs with the same name and namespace.
func (m *Minogrpc) CreateRPC(name string, h mino.Handler, f serde.Factory) (mino.RPC, error) {
	uri := append(append([]string{}, m.segments...), name)

	rpc := &RPC{
		uri:     strings.Join(uri, "/"),
		overlay: m.overlay,
		factory: f,
	}

	for _, segment := range uri {
		ok := segmentMatch.MatchString(segment)
		if !ok {
			return nil, xerrors.Errorf("invalid segment in uri '%s': '%s'", rpc.uri, segment)
		}
	}

	_, found := m.endpoints[rpc.uri]
	if found {
		return nil, xerrors.Errorf("rpc '%s' already exists", rpc.uri)
	}

	m.endpoints[rpc.uri] = &Endpoint{
		Handler: h,
		Factory: f,
		streams: make(map[string]session.Session),
	}

	return rpc, nil
}

// String implements fmt.Stringer. It prints a short description of the
// instance.
func (m *Minogrpc) String() string {
	return fmt.Sprintf("mino[%v]", m.overlay.myAddr)
}

// GetTrafficWatcher returns the traffic watcher.
func (m *Minogrpc) GetTrafficWatcher() traffic.Watcher {
	return traffic.GlobalWatcher
}

// Listen starts the server. It waits for the go routine to start before
// returning.
func (m *Minogrpc) listen(socket net.Listener) {
	go func() {
		defer m.closer.Done()

		close(m.started)

		err := m.server.Serve(socket)
		if err != nil {
			m.closing <- xerrors.Errorf("failed to serve: %v", err)
		}

		close(m.closing)
	}()

	// Force the go routine to be executed before returning which means the
	// server has well started after that point.
	<-m.started
}

// decorateServerTrace adds the protocol tag and the streamID tag to a server
// side trace.
func decorateServerTrace(ctx context.Context, span opentracing.Span, method string,
	req, resp interface{}, grpcError error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return
	}

	protocol := getOrEmpty(md, tracing.ProtocolTag)
	if protocol != "" {
		span.SetTag(tracing.ProtocolTag, protocol)
	}

	streamID := getOrEmpty(md, headerStreamIDKey)
	if streamID != "" {
		span.SetTag(headerStreamIDKey, streamID)
	}
}
