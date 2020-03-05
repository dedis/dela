// Package minogrpc is an implementation of MINO using gRPC to communicate
// over the network.
// This package implements the interfaces defined by Mino
package minogrpc

import (
	fmt "fmt"

	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=plugins=grpc:./ ./overlay.proto

// Minogrpc represents a grpc service restricted to a namespace
type Minogrpc struct {
	server    Server
	namespace string
}

// Address returns the address of the server
func (m Minogrpc) Address() *mino.Address {
	return m.server.addr
}

// MakeNamespace creates a new Minogrpc struct that has the specified namespace.
// This namespace is further used to scope newly created RPCs.
//
// TODO: decide if a server can evolve through multiple different namespaces or
// if a pre-condition of this function is to have an empty namespace.
func (m Minogrpc) MakeNamespace(namespace string) (mino.Mino, error) {
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

// MakeMinoGrpc sets up the grpc and http servers. It does not start the
// server. Identifier must be an address with a port, something like
// 127.0.0.1:3333
//
// TODO: use a different type of argument for identifier, maybe net/url ?
func MakeMinoGrpc(identifier string) (*Minogrpc, error) {
	addr := &mino.Address{
		Id: identifier,
	}

	server, err := CreateServer(addr)
	if err != nil {
		return nil, xerrors.Errorf("failed to create server: %v", err)
	}

	err = server.StartServer()
	if err != nil {
		return nil, xerrors.Errorf("failed to start the server: %v", err)
	}

	peer := Peer{
		Address:     server.listener.Addr().String(),
		Certificate: server.cert.Leaf,
	}
	server.neighbours[identifier] = peer

	minoGrpc := &Minogrpc{
		server:    *server,
		namespace: "",
	}

	return minoGrpc, err
}
