package debugsync

import (
	"sync"
)

type Mutex struct {
	mutex     sync.Mutex
	unlocking chan struct{}
}

func (m *Mutex) Lock() {
	locking := startLockTimer("RWMutex timed out when acquiring lock")
	m.mutex.Lock()
	close(locking)

	m.unlocking = startLockTimer("RWMutex timed out before releasing lock")
}

func (m *Mutex) TryLock() bool {
	locked := m.mutex.TryLock()

	if locked {
		m.unlocking = startLockTimer("RWMutex timed out before releasing lock")
	}

	return locked
}

func (m *Mutex) Unlock() {
	close(m.unlocking)
	m.mutex.Unlock()
}
