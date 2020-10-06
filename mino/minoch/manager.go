// This file implements a manager that will connect the different instances of
// Minoch so that they can communicate between each others.
//
// Documentation Last Review: 06.10.2020
//

package minoch

import (
	"sync"

	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

// Manager manages the communication between the local instances of Mino.
type Manager struct {
	sync.Mutex
	instances map[string]*Minoch
}

// NewManager creates a new empty manager.
func NewManager() *Manager {
	return &Manager{
		instances: make(map[string]*Minoch),
	}
}

func (m *Manager) get(a mino.Address) (*Minoch, error) {
	m.Lock()
	defer m.Unlock()

	addr, ok := a.(address)
	if !ok {
		return nil, xerrors.Errorf("invalid address type '%T'", a)
	}

	peer, ok := m.instances[addr.id]
	if !ok {
		return nil, xerrors.Errorf("address <%s> not found", addr.id)
	}

	return peer, nil
}

func (m *Manager) insert(inst mino.Mino) error {
	instance, ok := inst.(*Minoch)
	if !ok {
		return xerrors.Errorf("invalid instance type '%T'", inst)
	}

	if instance.identifier == "" {
		return xerrors.New("cannot have an empty identifier")
	}

	m.Lock()
	defer m.Unlock()

	_, found := m.instances[instance.identifier]
	if found {
		return xerrors.Errorf("identifier <%s> already exists", instance.identifier)
	}

	m.instances[instance.identifier] = instance

	return nil
}
