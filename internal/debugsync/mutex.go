package debugsync

import (
	"go.dedis.ch/dela"
	"runtime/debug"
	"sync"
)

// A Mutex is a mutual exclusion lock.
// The zero value for a Mutex is an unlocked mutex.
//
// A Mutex must not be copied after first use.
//
// In the terminology of the Go memory model,
// the n'th call to Unlock “synchronizes before” the m'th call to Lock
// for any n < m.
// A successful call to TryLock is equivalent to a call to Lock.
// A failed call to TryLock does not establish any “synchronizes before”
// relation at all.
type Mutex struct {
	mutex     sync.Mutex
	unlocking chan struct{}
}

// Lock locks m.
// If the lock is already in use, the calling goroutine
// blocks until the mutex is available.
func (m *Mutex) Lock() {
	dela.Logger.Debug().Msg("Locking")
	locking := startLockTimer("RWMutex timed out when acquiring lock", debug.Stack())
	m.mutex.Lock()
	close(locking)

	m.unlocking = startLockTimer("RWMutex timed out before releasing lock", debug.Stack())
}

// TryLock tries to lock m and reports whether it succeeded.
//
// Note that while correct uses of TryLock do exist, they are rare,
// and use of TryLock is often a sign of a deeper problem
// in a particular use of mutexes.
func (m *Mutex) TryLock() bool {
	locked := m.mutex.TryLock()

	if locked {
		m.unlocking = startLockTimer("RWMutex timed out before releasing lock", debug.Stack())
	}

	return locked
}

// Unlock unlocks m.
// It is a run-time error if m is not locked on entry to Unlock.
//
// A locked Mutex is not associated with a particular goroutine.
// It is allowed for one goroutine to lock a Mutex and then
// arrange for another goroutine to unlock it.
func (m *Mutex) Unlock() {
	close(m.unlocking)
	m.mutex.Unlock()
}
