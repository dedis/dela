package debugsync

import "sync"

type RWMutex struct {
	mutex     sync.RWMutex
	wg        sync.WaitGroup
	wgStarted bool
	unlocking chan struct{}
}

func (m *RWMutex) Lock() {
	locking := startLockTimer("RWMutex timed out when acquiring lock")
	m.mutex.Lock()
	close(locking)

	m.unlocking = startLockTimer("RWMutex timed out before releasing lock")
}

func (m *RWMutex) TryLock() bool {
	locked := m.mutex.TryLock()

	if locked {
		m.unlocking = startLockTimer("RWMutex timed out before releasing lock")
	}

	return locked
}

func (m *RWMutex) Unlock() {
	close(m.unlocking)
	m.mutex.Unlock()
}

func (m *RWMutex) RLock() {
	locking := startLockTimer("RWMutex timed out when acquiring RLock")
	m.mutex.RLock()
	close(locking)

	m.startRLockTimer("RMutex timed out before releasing RLock")
}

func (m *RWMutex) TryRLock() bool {
	locked := m.mutex.TryRLock()
	if locked {
		m.startRLockTimer("RMutex timed out before releasing RLock")
	}
	return locked
}

func (m *RWMutex) RUnlock() {
	m.wg.Done()
	m.mutex.RUnlock()
}

func (m *RWMutex) startRLockTimer(msg string) {
	m.wg.Add(1)

	if m.wgStarted {
		return
	}

	m.wgStarted = true

	done := startLockTimer(msg)
	go func() {
		m.wg.Wait()
		close(done)
	}()
}
