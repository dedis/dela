// Package minoch is an implementation of Mino that is using channels and a
// local manager to exchange messages.
//
// Because it is using only Go channels to communicate, this implementation can
// only be used by multiple instances in the same process. Its usage is purely
// to simplify the writing of tests, therefore it also provides some additionnal
// functionalities like filters.
//
// A filter is called for any message incoming and it will determine if the
// instance should drop the message.
//
// Documentation Last Review: 06.10.2020
//
package minoch

import (
	"fmt"
	"sync"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
)

// Filter is a function called for any request to an RPC which will drop it if
// it returns false.
type Filter func(mino.Request) bool

// Minoch is an implementation of the Mino interface using channels. Each
// instance must have a unique string assigned to it.
//
// - implements mino.Mino
type Minoch struct {
	sync.Mutex

	manager    *Manager
	identifier string
	path       string
	rpcs       map[string]*RPC
	context    serde.Context
	filters    []Filter
}

// NewMinoch creates a new instance of a local Mino instance.
func NewMinoch(manager *Manager, identifier string) *Minoch {
	inst := &Minoch{
		manager:    manager,
		identifier: identifier,
		path:       "",
		rpcs:       make(map[string]*RPC),
		context:    json.NewContext(),
	}

	err := manager.insert(inst)
	if err != nil {
		panic("manager refused: " + err.Error())
	}

	dela.Logger.Trace().Msgf("New instance with identifier %s", identifier)

	return inst
}

// GetAddressFactory implements mino.Mino. It returns the address factory.
func (m *Minoch) GetAddressFactory() mino.AddressFactory {
	return AddressFactory{}
}

// GetAddress implements mino.Mino. It returns the address that other
// participants should use to contact this instance.
func (m *Minoch) GetAddress() mino.Address {
	return address{id: m.identifier}
}

// AddFilter adds the filter to all of the RPCs. This must be called before
// receiving requests.
func (m *Minoch) AddFilter(filter Filter) {
	m.filters = append(m.filters, filter)

	for _, rpc := range m.rpcs {
		rpc.filters = m.filters
	}
}

// MakeNamespace implements mino.Mino. It returns an instance restricted to the
// namespace.
func (m *Minoch) MakeNamespace(path string) (mino.Mino, error) {
	newMinoch := &Minoch{
		manager:    m.manager,
		identifier: m.identifier,
		path:       fmt.Sprintf("%s/%s", m.path, path),
		rpcs:       m.rpcs,
	}

	return newMinoch, nil
}

// MakeRPC implements mino.Mino. It creates an RPC that can send to and receive
// from the unique path.
func (m *Minoch) MakeRPC(name string, h mino.Handler, f serde.Factory) (mino.RPC, error) {
	rpc := &RPC{
		manager: m.manager,
		addr:    m.GetAddress(),
		path:    fmt.Sprintf("%s/%s", m.path, name),
		h:       h,
		context: m.context,
		factory: f,
		filters: m.filters,
	}

	m.Lock()
	m.rpcs[rpc.path] = rpc
	m.Unlock()

	return rpc, nil
}
