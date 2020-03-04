// Package minogrpc is an implementation of MINO using gRPC to communicate
// over the network.
// This package implements the interfaces defined by Mino
package minogrpc

import (
	fmt "fmt"
	"log"

	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
	grpc "google.golang.org/grpc"
)

//go:generate protoc -I ./ --go_out=plugins=grpc:./ ./overlay.proto

// Minogrpc ...
type Minogrpc struct {
	server    Server
	namespace string
}

// Address ...
func (m Minogrpc) Address() *mino.Address {
	return m.server.addr
}

// MakeNamespace creates a new Minogrpc struct that has the specified path
func (m Minogrpc) MakeNamespace(namespace string) (mino.Mino, error) {
	newM := Minogrpc{
		server:    m.server,
		namespace: namespace,
	}
	return newM, nil
}

// MakeRPC ...
func (m Minogrpc) MakeRPC(name string, h mino.Handler) (mino.RPC, error) {
	URI := fmt.Sprintf("%s/%s", m.namespace, name)
	rpc := RPC{
		handler: h,
		srv:     m.server,
		URI:     URI,
	}

	m.server.handlers[URI] = h

	return rpc, nil
}

// MakeMinoGrpc ...
// identifier must be an address with a port, something like 127.0.0.1:3333
func MakeMinoGrpc(identifier string) (*Minogrpc, error) {
	addr := &mino.Address{
		Id: identifier,
	}

	cert, err := makeCertificate()
	if err != nil {
		return nil, xerrors.Errorf("failed to create certificate: %v", err)
	}

	srv := grpc.NewServer()

	server := Server{
		grpcSrv:    srv,
		cert:       cert,
		addr:       addr,
		listener:   nil,
		StartChan:  make(chan struct{}),
		neighbours: make(map[string]Peer),
		handlers:   make(map[string]mino.Handler),
	}

	RegisterOverlayServer(srv, &overlayService{handlers: server.handlers})

	go func() {
		err := server.Serve()
		// TODO: better handle this error
		log.Fatal("failed to start server: ", err)
	}()

	<-server.StartChan

	peer := Peer{
		Address:     server.listener.Addr().String(),
		Certificate: server.cert.Leaf,
	}
	server.neighbours[identifier] = peer

	minoGrpc := &Minogrpc{
		server:    server,
		namespace: "",
	}

	return minoGrpc, err
}
