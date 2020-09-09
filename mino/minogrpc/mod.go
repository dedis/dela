// Package minogrpc is an implementation of MINO using gRPC to communicate
// over the network.
// This package implements the interfaces defined by Mino
package minogrpc

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sync"
	"time"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

//go:generate protoc -I ./ --go_out=plugins=grpc:./ ./overlay.proto

const (
	orchestratorDescription = "Orchestrator"
	orchestratorCode        = "\ue000"
)

var (
	namespaceMatch        = regexp.MustCompile("^[a-zA-Z0-9]+$")
	defaultAddressFactory = AddressFactory{}
)

// rootAddress is the address of the orchestrator of a protocol. When Stream is
// called, the caller takes this address so that participants know how to route
// message to it.
//
// - implements mino.Address
type rootAddress struct{}

func newRootAddress() rootAddress {
	return rootAddress{}
}

// Equal implements mino.Address. It returns true if the other address is also a
// root address.
func (a rootAddress) Equal(other mino.Address) bool {
	addr, ok := other.(rootAddress)
	return ok && a == addr
}

// MarshalText implements mino.Address. It returns a buffer that uses the
// private area of Unicode to define the root.
func (a rootAddress) MarshalText() ([]byte, error) {
	return []byte(orchestratorCode), nil
}

// String implements fmt.Stringer. It returns a string representation of the
// address.
func (a rootAddress) String() string {
	return orchestratorDescription
}

// address is a representation of the network address of a participant.
//
// - implements mino.Address
type address struct {
	host string
}

// GetDialAddress returns a string formatted to be understood by grpc.Dial()
// functions.
func (a address) GetDialAddress() string {
	// TODO: check the DNS resolver thing.
	return a.host
}

// Equal implements mino.Address. It returns true if both addresses points to
// the same participant.
func (a address) Equal(other mino.Address) bool {
	addr, ok := other.(address)
	return ok && addr == a
}

// MarshalText implements mino.Address. It returns the text format of the
// address that can later be deserialized.
func (a address) MarshalText() ([]byte, error) {
	return []byte(a.host), nil
}

// String implements fmt.Stringer. It returns a string representation of the
// address.
func (a address) String() string {
	return a.host
}

// AddressFactory implements mino.AddressFactory
type AddressFactory struct {
	serde.Factory
}

// FromText implements mino.AddressFactory. It returns an instance of an
// address from a byte slice.
func (f AddressFactory) FromText(text []byte) mino.Address {
	if string(text) == orchestratorCode {
		return newRootAddress()
	}

	return address{host: string(text)}
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
	sync.Mutex
	Handler mino.Handler
	Factory serde.Factory
	streams map[string]session.Session
}

// Minogrpc represents a grpc service restricted to a namespace
//
// - implements mino.Mino
// - implements fmt.Stringer
type Minogrpc struct {
	overlay
	url       *url.URL
	server    *grpc.Server
	namespace string
	endpoints map[string]*Endpoint
	started   chan struct{}
	closer    *sync.WaitGroup
	closing   chan error
}

// NewMinogrpc creates and starts a new instance. The path should be a
// resolvable host.
func NewMinogrpc(path string, port uint16, router router.Router) (*Minogrpc, error) {
	url, err := url.Parse(fmt.Sprintf("//%s:%d", path, port))
	if err != nil {
		return nil, xerrors.Errorf("couldn't parse url: %v", err)
	}

	me := address{host: url.Host}

	o, err := newOverlay(me, router, defaultAddressFactory, json.NewContext())
	if err != nil {
		return nil, xerrors.Errorf("couldn't make overlay: %v", err)
	}

	creds := credentials.NewServerTLSFromCert(o.GetCertificate())
	server := grpc.NewServer(grpc.Creds(creds))

	m := &Minogrpc{
		overlay:   o,
		url:       url,
		server:    server,
		namespace: "",
		endpoints: make(map[string]*Endpoint),
		started:   make(chan struct{}),
		closer:    &sync.WaitGroup{},
		closing:   make(chan error, 1),
	}

	// Counter needs to be >=1 for asynchronous call to Add.
	m.closer.Add(1)

	ptypes.RegisterOverlayServer(server, &overlayServer{
		overlay:   o,
		endpoints: m.endpoints,
		closer:    m.closer,
	})

	err = m.Listen()
	if err != nil {
		return nil, xerrors.Errorf("couldn't start the server: %v", err)
	}

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

// Listen starts the server. It waits for the go routine to start before
// returning.
func (m *Minogrpc) Listen() error {
	// TODO: bind 0.0.0.0:PORT => get port from address.
	lis, err := net.Listen("tcp4", m.url.Host)
	if err != nil {
		return xerrors.Errorf("failed to listen: %v", err)
	}

	go func() {
		close(m.started)

		err := m.server.Serve(lis)
		if err != nil {
			m.closing <- xerrors.Errorf("failed to serve: %v", err)
		}

		close(m.closing)
	}()

	// Force the go routine to be executed before returning which means the
	// server has well started after that point.
	<-m.started

	return nil
}

// GracefulClose first stops the grpc server then waits for the remaining
// handlers to close.
func (m *Minogrpc) GracefulClose() error {
	select {
	case <-m.started:
	default:
		return xerrors.New("server should be listening before trying to close")
	}

	m.server.GracefulStop()

	m.closer.Done()
	m.closer.Wait()

	err := <-m.closing
	if err != nil {
		return xerrors.Errorf("failed to stop gracefully: %v", err)
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
