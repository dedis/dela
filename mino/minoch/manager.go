package minoch

import (
	"sync"

	"golang.org/x/xerrors"
)

// Manager is an orchestrator to manage the communication between the local
// instances of Mino.
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

func (m *Manager) get(id string) *Minoch {
	m.Lock()
	defer m.Unlock()

	return m.instances[id]
}

func (m *Manager) insert(inst *Minoch) error {
	id := inst.GetAddress().String()
	if id == "" {
		return xerrors.New("identifier must not be empty")
	}

	m.Lock()
	defer m.Unlock()

	if _, ok := m.instances[id]; ok {
		return xerrors.New("identifier already exists")
	}

	m.instances[id] = inst

	return nil
}
