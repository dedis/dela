// Package minogrpc implements a network overlay using gRPC.
//
// Documentation Last Review: 07.10.2020
//
package minogrpc

import (
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"regexp"
	"sync"
	"time"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	namespaceMatch = regexp.MustCompile("^[a-zA-Z0-9]+$")
	addressFac     = session.AddressFactory{}
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

// Joinable is an extension of the mino.Mino interface to allow distant servers
// to join a network of participants.
type Joinable interface {
	mino.Mino

	// GetCertificate returns the certificate of the instance.
	GetCertificate() *tls.Certificate

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
	Join(addr, token string, certHash []byte) error
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
	namespace string
	endpoints map[string]*Endpoint
	started   chan struct{}
	closing   chan error
}

type minoTemplate struct {
	myAddr mino.Address
	router router.Router
	fac    mino.AddressFactory
	certs  certs.Storage
	secret interface{}
	public interface{}
	curve  elliptic.Curve
	random io.Reader
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

// NewMinogrpc creates and starts a new instance. it will try to listen for the
// address and returns an error if it fails.
func NewMinogrpc(addr net.Addr, router router.Router, opts ...Option) (*Minogrpc, error) {
	socket, err := net.Listen(addr.Network(), addr.String())
	if err != nil {
		return nil, xerrors.Errorf("failed to bind: %v", err)
	}

	tmpl := minoTemplate{
		myAddr: session.NewAddress(socket.Addr().String()),
		router: router,
		fac:    addressFac,
		certs:  certs.NewInMemoryStore(),
		curve:  elliptic.P521(),
		random: rand.Reader,
	}

	for _, opt := range opts {
		opt(&tmpl)
	}

	o, err := newOverlay(tmpl)
	if err != nil {
		socket.Close()

		return nil, xerrors.Errorf("overlay: %v", err)
	}

	creds := credentials.NewServerTLSFromCert(o.GetCertificate())
	server := grpc.NewServer(grpc.Creds(creds))

	m := &Minogrpc{
		overlay:   o,
		server:    server,
		namespace: "",
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

// Stop stops the server immediatly.
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

	return nil
}

// MakeNamespace implements mino.Mino. It returns a instance that extends the
// current namespace with the one provided in parameter. The namespace can not
// be empty an should match [a-zA-Z0-9]+
func (m *Minogrpc) MakeNamespace(namespace string) (mino.Mino, error) {
	if namespace == "" {
		return nil, xerrors.Errorf("a namespace can not be empty")
	}

	ok := namespaceMatch.MatchString(namespace)
	if !ok {
		return nil, xerrors.Errorf("a namespace should match [a-zA-Z0-9]+, "+
			"but found '%s'", namespace)
	}

	newM := &Minogrpc{
		server:    m.server,
		overlay:   m.overlay,
		namespace: concatenateNamespace(m.namespace, namespace),
		endpoints: m.endpoints,
	}

	return newM, nil
}

// MakeRPC implements mino.Mino. It returns a newly created rpc with the
// provided name and reserved for the current namespace. When contacting distant
// peers, it will only talk to mirrored RPCs with the same name and namespace.
func (m *Minogrpc) MakeRPC(name string, h mino.Handler, f serde.Factory) (mino.RPC, error) {
	uri := concatenateNamespace(m.namespace, name)

	rpc := &RPC{
		uri:     uri,
		overlay: m.overlay,
		factory: f,
	}

	m.endpoints[uri] = &Endpoint{
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

func concatenateNamespace(base, segment string) string {
	return fmt.Sprintf("%s/%s", base, segment)
}
