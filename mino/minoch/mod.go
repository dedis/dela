// Package minoch is an implementation of MINO that is using channels and a
// local manager to exchange messages.
package minoch

import (
	"fmt"

	"go.dedis.ch/m"
	"go.dedis.ch/m/mino"
)

// Minoch is an implementation of the Mino interface using channels. Each
// instance must have a unique string assigned to it.
type Minoch struct {
	manager    *Manager
	identifier string
	path       string
	rpcs       map[string]RPC
}

// NewMinoch creates a new instance of a local Mino instance.
func NewMinoch(manager *Manager, identifier string) (*Minoch, error) {
	inst := &Minoch{
		manager:    manager,
		identifier: identifier,
		path:       "",
		rpcs:       make(map[string]RPC),
	}

	err := manager.insert(inst)
	if err != nil {
		return nil, err
	}

	m.Logger.Info().Msgf("New instance with identifier %s", identifier)

	return inst, nil
}

// Address returns the address that other participants should use to contact
// this instance.
func (m *Minoch) Address() *mino.Address {
	return &mino.Address{Id: m.identifier}
}

// MakePath returns an instance restricted to the path.
func (m *Minoch) MakePath(path string) (mino.Mino, error) {
	newMinoch := &Minoch{
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
		path:    fmt.Sprintf("%s/%s", m.path, name),
		h:       h,
	}
	m.rpcs[rpc.path] = rpc

	return rpc, nil
}
