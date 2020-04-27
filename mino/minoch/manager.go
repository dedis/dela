package minoch

import (
	"sync"

	"go.dedis.ch/fabric/mino"
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

func (m *Manager) get(addr mino.Address) *Minoch {
	m.Lock()
	defer m.Unlock()

	text, err := addr.MarshalText()
	if err != nil {
		return nil
	}

	return m.instances[string(text)]
}

func (m *Manager) insert(inst *Minoch) error {
	addr := inst.GetAddress().(address)

	text, err := addr.MarshalText()
	if err != nil {
		return xerrors.Errorf("couldn't marshal address: %v", err)
	}

	if string(text) == "" {
		return xerrors.New("can't have an empty marshaled address")
	}

	m.Lock()
	defer m.Unlock()

	_, found := m.instances[string(text)]
	if found {
		return xerrors.New("identifier already exists")
	}

	m.instances[string(text)] = inst

	return nil
}
