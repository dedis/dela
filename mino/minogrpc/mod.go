// Package minogrpc is an implementation of MINO using gRPC to communicate
// over the network.
// This package implements the interfaces defined by Mino
package minogrpc

import (
	"crypto/tls"
	"fmt"
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

const (
	orchestratorDescription = "Orchestrator"
	orchestratorCode        = "\ue000"
)

var (
	namespaceMatch        = regexp.MustCompile("^[a-zA-Z0-9]+$")
	defaultAddressFactory = AddressFactory{}
	orchestratorBytes     = []byte(orchestratorCode)
)

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

	GetCertificate() *tls.Certificate

	GetCertificateStore() certs.Storage

	GenerateToken(expiration time.Duration) string

	Join(addr, token string, digest []byte) error
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

// Minogrpc represents a grpc service restricted to a namespace
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
	me     mino.Address
	router router.Router
	fac    mino.AddressFactory
	certs  certs.Storage
	secret interface{}
	public interface{}
}

type Option func(*minoTemplate)

func WithStorage(certs certs.Storage) Option {
	return func(tmpl *minoTemplate) {
		tmpl.certs = certs
	}
}

func WithCertificateKey(secret, public interface{}) Option {
	return func(tmpl *minoTemplate) {
		tmpl.secret = secret
		tmpl.public = public
	}
}

// NewMinogrpc creates and starts a new instance. The path should be a
// resolvable host.
func NewMinogrpc(addr net.Addr, router router.Router, opts ...Option) (*Minogrpc, error) {
	socket, err := net.Listen(addr.Network(), addr.String())
	if err != nil {
		return nil, xerrors.Errorf("failed to bind: %v", err)
	}

	tmpl := minoTemplate{
		me:     address{host: socket.Addr().String()},
		router: router,
		fac:    AddressFactory{},
		certs:  certs.NewInMemoryStore(),
	}

	for _, opt := range opts {
		opt(&tmpl)
	}

	o, err := newOverlay(tmpl)
	if err != nil {
		return nil, xerrors.Errorf("couldn't make overlay: %v", err)
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

// GetAddressFactory implements Mino. It returns the address
// factory.
func (m *Minogrpc) GetAddressFactory() mino.AddressFactory {
	return defaultAddressFactory
}

// GetAddress implements Mino. It returns the address of the server.
func (m *Minogrpc) GetAddress() mino.Address {
	return m.overlay.me
}

// GenerateToken generates and returns a new token that will be valid for the
// given amount of time.
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

// MakeNamespace implements Mino. It creates a new Minogrpc
// struct that has the specified namespace. This namespace is further used to
// scope newly created RPCs. There can be multiple namespaces. If there is
// already a namespace, then the new one will be concatenated leading to
// namespace1/namespace2. A namespace can not be empty an should match
// [a-zA-Z0-9]+
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
		namespace: namespace,
		endpoints: m.endpoints,
	}
	return newM, nil
}

// MakeRPC implements Mino. It registers the handler using a uniq URI of
// form "namespace/name". It returns a struct that allows client to call the
// RPC.
func (m *Minogrpc) MakeRPC(name string, h mino.Handler, f serde.Factory) (mino.RPC, error) {
	rpc := &RPC{
		uri:     fmt.Sprintf("%s/%s", m.namespace, name),
		overlay: m.overlay,
		factory: f,
	}

	uri := fmt.Sprintf("%s/%s", m.namespace, name)
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
	return fmt.Sprintf("%v", m.overlay.me)
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
