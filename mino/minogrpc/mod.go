// Package minogrpc is an implementation of MINO using gRPC to communicate
// over the network.
// This package implements the interfaces defined by Mino
package minogrpc

import (
	"go.dedis.ch/fabric/mino"
)

//go:generate protoc -I ./ --go_out=plugins=grpc:./ ./overlay.proto

// Minogrpc ...
type Minogrpc struct {
	addr *mino.Address
}

// Address ...
func (m Minogrpc) Address() *mino.Address {
	return m.addr
}

// MakePath ...
func (m Minogrpc) MakePath(ns string) (mino.Mino, error) {
	return nil, nil
}

// MakeRPC ...
func (m Minogrpc) MakeRPC(name string, h mino.Handler) (mino.RPC, error) {
	return nil, nil
}

// MakeMinoGrpc ...
func MakeMinoGrpc(identifier string) *Minogrpc {
	return &Minogrpc{
		addr: &mino.Address{
			Id: identifier,
		},
	}
}
