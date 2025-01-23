// This file implements a manager that will connect the different instances of
// Minows so that they can communicate between each others.
//

package minows

import (
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

// Manager manages the communication between the local instances of Mino.
type Manager struct {
	sync.Mutex
	instances map[peer.ID]*Minows
}

// NewManager creates a new empty manager.
func NewManager() *Manager {
	return &Manager{
		instances: make(map[peer.ID]*Minows),
	}
}

func (m *Manager) get(a mino.Address) (*Minows, error) {
	m.Lock()
	defer m.Unlock()

	addr, ok := a.(address)
	if !ok {
		return nil, xerrors.Errorf("invalid address type '%T'", a)
	}

	instance, ok := m.instances[addr.identity]
	if !ok {
		return nil, xerrors.Errorf("address <%s> not found", addr.identity)
	}

	return instance, nil
}

func (m *Manager) insert(inst mino.Mino) error {
	instance, ok := inst.(*Minows)
	if !ok {
		return xerrors.Errorf("invalid instance type '%T'", inst)
	}

	if instance.myAddr.identity == "" {
		return xerrors.New("cannot have an empty identifier")
	}

	m.Lock()
	defer m.Unlock()

	_, found := m.instances[instance.myAddr.identity]
	if found {
		return xerrors.Errorf("identifier <%s> already exists", instance.myAddr.identity)
	}

	m.instances[instance.myAddr.identity] = instance

	return nil
}
