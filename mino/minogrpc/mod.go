// Package minogrpc is an implementation of MINO using gRPC to communicate
// over the network.
// This package implements the interfaces defined by Mino
package minogrpc

import (
	"crypto/x509"
	fmt "fmt"
	"regexp"

	"net/url"

	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minogrpc/routing"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=plugins=grpc:./ ./overlay.proto

var (
	namespaceMatch        = regexp.MustCompile("^[a-zA-Z0-9]+$")
	defaultAddressFactory = AddressFactory{}
)

// Minogrpc represents a grpc service restricted to a namespace
type Minogrpc struct {
	server    *Server
	namespace string
}

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

// NewMinogrpc sets up the grpc and http servers. URL should
func NewMinogrpc(serverURL *url.URL, rf routing.Factory) (*Minogrpc, error) {

	if serverURL.Host == "" {
		return nil, xerrors.Errorf("host URL is invalid: empty host for '%s'. "+
			"Hint: the url must be created using an absolute path, "+
			"like //127.0.0.1:3333", serverURL)
	}

	addr := address{
		id: serverURL.Host,
	}

	server, err := NewServer(addr, rf)
	if err != nil {
		return nil, xerrors.Errorf("failed to create server: %v", err)
	}

	server.nodesCerts.Store(serverURL.Host, server.cert.Leaf)

	server.StartServer()

	return &Minogrpc{
		server:    server,
		namespace: "",
	}, err
}

// GetAddressFactory implements Mino. It returns the address
// factory.
func (m Minogrpc) GetAddressFactory() mino.AddressFactory {
	return defaultAddressFactory
}

// GetAddress implements Mino. It returns the address of the server
func (m Minogrpc) GetAddress() mino.Address {
	return m.server.addr
}

// MakeNamespace implements Mino. It creates a new Minogrpc
// struct that has the specified namespace. This namespace is further used to
// scope newly created RPCs. There can be multiple namespaces. If there is
// already a namespace, then the new one will be concatenated leading to
// namespace1/namespace2. A namespace can not be empty an should match
// [a-zA-Z0-9]+
func (m Minogrpc) MakeNamespace(namespace string) (mino.Mino, error) {
	if namespace == "" {
		return nil, xerrors.Errorf("a namespace can not be empty")
	}

	ok := namespaceMatch.MatchString(namespace)
	if !ok {
		return nil, xerrors.Errorf("a namespace should match [a-zA-Z0-9]+, "+
			"but found '%s'", namespace)
	}

	newM := Minogrpc{
		server:    m.server,
		namespace: namespace,
	}
	return newM, nil
}

// MakeRPC implements Mino. It registers the handler using a uniq URI of
// form "namespace/name". It returns a struct that allows client to call the
// RPC.
func (m Minogrpc) MakeRPC(name string, h mino.Handler) (mino.RPC, error) {
	URI := fmt.Sprintf("%s/%s", m.namespace, name)
	rpc := &RPC{
		encoder:        encoding.NewProtoEncoder(),
		handler:        h,
		srv:            m.server,
		uri:            URI,
		routingFactory: m.server.routingFactory,
	}

	m.server.handlers[URI] = h

	return rpc, nil
}

// AddCertificate populates the list of public know certificates of the server
func (m Minogrpc) AddCertificate(u *url.URL, cert x509.Certificate) error {
	if u.Host == "" {
		return xerrors.Errorf("host URL is invalid: empty host for '%s'. "+
			"Hint: the url must be created using an absolute path, "+
			"like //127.0.0.1:3333", u)
	}

	m.server.addCertificate(u.Host, &cert)

	return nil
}

// GetPublicCertificate returns the public certificate of the server
func (m Minogrpc) GetPublicCertificate() x509.Certificate {
	return *m.server.cert.Leaf
}
