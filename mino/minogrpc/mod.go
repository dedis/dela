// Package minogrpc is an implementation of MINO using gRPC to communicate
// over the network.
// This package implements the interfaces defined by Mino
package minogrpc

import (
	"crypto/tls"
	fmt "fmt"
	"net"
	"net/url"
	"regexp"

	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minogrpc/routing"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

//go:generate protoc -I ./ --go_out=plugins=grpc:./ ./overlay.proto

var (
	namespaceMatch        = regexp.MustCompile("^[a-zA-Z0-9]+$")
	defaultAddressFactory = AddressFactory{}
)

// address implements mino.Address
// TODO: improve to support internet addresses.
type address struct {
	id string
}

func (a address) Equal(other mino.Address) bool {
	addr, ok := other.(address)
	if ok {
		return ok && addr.id == a.id
	}
	addrPtr, ok := other.(*address)
	return ok && addrPtr.id == a.id
}

// MarshalText implements mino.Address
func (a address) MarshalText() ([]byte, error) {
	return []byte(a.id), nil
}

// String implements mino.Address
func (a address) String() string {
	return a.id
}

// AddressFactory implements mino.AddressFactory
type AddressFactory struct{}

// FromText implements mino.AddressFactory. It returns an instance of an
// address from a byte slice.
func (f AddressFactory) FromText(text []byte) mino.Address {
	return address{id: string(text)}
}

// Minogrpc represents a grpc service restricted to a namespace
type Minogrpc struct {
	server    *grpc.Server
	overlay   overlay
	namespace string
	handlers  map[string]mino.Handler
}

// NewMinogrpc sets up the grpc and http servers. URL should
func NewMinogrpc(path string, port uint16, rf routing.Factory) (*Minogrpc, error) {
	url, err := url.Parse(fmt.Sprintf("//%s:%d", path, port))
	if err != nil {
		return nil, xerrors.Errorf("couldn't parse url: %v", err)
	}

	me := address{id: url.Host}

	o, err := newOverlay(me, rf)
	if err != nil {
		return nil, xerrors.Errorf("couldn't make overlay: %v", err)
	}

	creds := credentials.NewServerTLSFromCert(o.GetCertificate())
	server := grpc.NewServer(grpc.Creds(creds))

	m := &Minogrpc{
		server:    server,
		overlay:   o,
		namespace: "",
		handlers:  make(map[string]mino.Handler),
	}

	RegisterOverlayServer(server, overlayServer{
		overlay:  o,
		handlers: m.handlers,
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

// GetAddress implements Mino. It returns the address of the server
func (m *Minogrpc) GetAddress() mino.Address {
	return m.overlay.me
}

// GetCertificate returns the public certificate of the server
func (m *Minogrpc) GetCertificate() *tls.Certificate {
	return m.overlay.GetCertificate()
}

// AddCertificate populates the list of public know certificates of the server
func (m *Minogrpc) AddCertificate(addr mino.Address, cert *tls.Certificate) error {
	m.overlay.certs.Store(addr, cert)

	return nil
}

// Listen starts the server.
func (m *Minogrpc) Listen() error {
	lis, err := net.Listen("tcp4", m.GetAddress().String())
	if err != nil {
		return xerrors.Errorf("failed to listen: %v", err)
	}

	go func() {
		err = m.server.Serve(lis)
		if err != nil {
			fabric.Logger.Err(err).Send()
		}
	}()

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
		handlers:  m.handlers,
	}
	return newM, nil
}

// MakeRPC implements Mino. It registers the handler using a uniq URI of
// form "namespace/name". It returns a struct that allows client to call the
// RPC.
func (m *Minogrpc) MakeRPC(name string, h mino.Handler) (mino.RPC, error) {
	rpc := &RPC{
		uri:     fmt.Sprintf("%s/%s", m.namespace, name),
		overlay: m.overlay,
	}

	uri := fmt.Sprintf("%s/%s", m.namespace, name)
	m.handlers[uri] = h

	return rpc, nil
}
