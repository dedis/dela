package minoch

import (
	"sync"

	"golang.org/x/xerrors"
)

// Manager is an orchestrator to manage the communication between the local
// instances of Mino.
type Manager struct {
	sync.Mutex
	instances map[address]*Minoch
}

// NewManager creates a new empty manager.
func NewManager() *Manager {
	return &Manager{
		instances: make(map[address]*Minoch),
	}
}

func (m *Manager) get(addr address) *Minoch {
	m.Lock()
	defer m.Unlock()

	return m.instances[addr]
}

func (m *Manager) insert(inst *Minoch) error {
	addr := inst.GetAddress().(address)
	if addr.String() == "" {
		return xerrors.New("identifier must not be empty")
	}

	m.Lock()
	defer m.Unlock()

	if _, ok := m.instances[addr]; ok {
		return xerrors.New("identifier already exists")
	}

	m.instances[addr] = inst

	return nil
}
