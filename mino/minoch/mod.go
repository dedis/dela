// Package minoch is an implementation of MINO that is using channels and a
// local manager to exchange messages.
package minoch

import (
	"fmt"
	"sync"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/mino"
)

// Minoch is an implementation of the Mino interface using channels. Each
// instance must have a unique string assigned to it.
type Minoch struct {
	sync.Mutex
	manager    *Manager
	encoder    encoding.ProtoMarshaler
	identifier string
	path       string
	rpcs       map[string]RPC
}

// NewMinoch creates a new instance of a local Mino instance.
func NewMinoch(manager *Manager, identifier string) (*Minoch, error) {
	inst := &Minoch{
		manager:    manager,
		encoder:    encoding.NewProtoEncoder(),
		identifier: identifier,
		path:       "",
		rpcs:       make(map[string]RPC),
	}

	err := manager.insert(inst)
	if err != nil {
		return nil, err
	}

	dela.Logger.Trace().Msgf("New instance with identifier %s", identifier)

	return inst, nil
}

// GetAddressFactory returns the address factory.
func (m *Minoch) GetAddressFactory() mino.AddressFactory {
	return AddressFactory{}
}

// GetAddress returns the address that other participants should use to contact
// this instance.
func (m *Minoch) GetAddress() mino.Address {
	return address{id: m.identifier}
}

// MakeNamespace returns an instance restricted to the namespace.
func (m *Minoch) MakeNamespace(path string) (mino.Mino, error) {
	newMinoch := &Minoch{
		manager:    m.manager,
		identifier: m.identifier,
		path:       fmt.Sprintf("%s/%s", m.path, path),
		rpcs:       m.rpcs,
	}

	return newMinoch, nil
}

// MakeRPC creates an RPC that can send to and receive from the unique path.
func (m *Minoch) MakeRPC(name string, h mino.Handler) (mino.RPC, error) {
	rpc := RPC{
		manager: m.manager,
		encoder: m.encoder,
		addr:    m.GetAddress(),
		path:    fmt.Sprintf("%s/%s", m.path, name),
		h:       h,
	}

	m.Lock()
	m.rpcs[rpc.path] = rpc
	m.Unlock()

	return rpc, nil
}
