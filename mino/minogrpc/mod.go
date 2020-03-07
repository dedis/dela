// Package minogrpc is an implementation of MINO using gRPC to communicate
// over the network.
// This package implements the interfaces defined by Mino
package minogrpc

import (
	fmt "fmt"
	"regexp"

	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=plugins=grpc:./ ./overlay.proto

var namespaceMatch = regexp.MustCompile("^[a-zA-Z0-9]+$")

// Minogrpc represents a grpc service restricted to a namespace
type Minogrpc struct {
	server    Server
	namespace string
}

// TODO: improve to support internet addresses.
type address struct {
	id string
}

func (a address) String() string {
	return a.id
}

// NewMinogrpc sets up the grpc and http servers. It does not start the
// server. Identifier must be an address with a port, something like
// 127.0.0.1:3333
//
// TODO: use a different type of argument for identifier, maybe net/url ?
func NewMinogrpc(identifier string) (Minogrpc, error) {
	minoGrpc := Minogrpc{}

	addr := address{
		id: identifier,
	}

	server, err := CreateServer(addr)
	if err != nil {
		return minoGrpc, xerrors.Errorf("failed to create server: %v", err)
	}

	err = server.StartServer()
	if err != nil {
		return minoGrpc, xerrors.Errorf("failed to start the server: %v", err)
	}

	peer := Peer{
		Address:     server.listener.Addr().String(),
		Certificate: server.cert.Leaf,
	}
	server.neighbours[identifier] = peer

	minoGrpc.server = *server
	minoGrpc.namespace = ""

	return minoGrpc, err
}

// GetAddressFactory returns the address factory.
// TODO: need implementation
func (m Minogrpc) GetAddressFactory() mino.AddressFactory {
	return nil
}

// GetAddress returns the address of the server
func (m Minogrpc) GetAddress() mino.Address {
	return m.server.addr
}

// MakeNamespace creates a new Minogrpc struct that has the specified namespace.
// This namespace is further used to scope newly created RPCs. There can be
// multiple namespaces. If there is already a namespace, then the new one will
// be concatenated leading to namespace1/namespace2. A namespace can not be
// empty an should match [a-zA-Z0-9]+
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

// MakeRPC registers the handler using a uniq URI of form "namespace/name". It
// returns a struct that allows client to call the RPC.
func (m Minogrpc) MakeRPC(name string, h mino.Handler) (mino.RPC, error) {
	URI := fmt.Sprintf("%s/%s", m.namespace, name)
	rpc := RPC{
		handler: h,
		srv:     m.server,
		uri:     URI,
	}

	m.server.handlers[URI] = h

	return rpc, nil
}
