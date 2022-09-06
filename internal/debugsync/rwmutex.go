package debugsync

import (
	"runtime/debug"
	"sync"
)

// A RWMutex is a reader/writer mutual exclusion lock.
// The lock can be held by an arbitrary number of readers or a single writer.
// The zero value for a RWMutex is an unlocked mutex.
//
// A RWMutex must not be copied after first use.
//
// If a goroutine holds a RWMutex for reading and another goroutine might
// call Lock, no goroutine should expect to be able to acquire a read lock
// until the initial read lock is released. In particular, this prohibits
// recursive read locking. This is to ensure that the lock eventually becomes
// available; a blocked Lock call excludes new readers from acquiring the
// lock.
//
// In the terminology of the Go memory model,
// the n'th call to Unlock “synchronizes before” the m'th call to Lock
// for any n < m, just as for Mutex.
// For any call to RLock, there exists an n such that
// the n'th call to Unlock “synchronizes before” that call to RLock,
// and the corresponding call to RUnlock “synchronizes before”
// the n+1'th call to Lock.
type RWMutex struct {
	mutex     sync.RWMutex
	wg        sync.WaitGroup
	wgStarted bool
	unlocking chan struct{}
}

// Lock locks rw for writing.
// If the lock is already locked for reading or writing,
// Lock blocks until the lock is available.
func (m *RWMutex) Lock() {
	locking := startLockTimer("RWMutex timed out when acquiring lock", debug.Stack())
	m.mutex.Lock()
	close(locking)

	m.unlocking = startLockTimer("RWMutex timed out before releasing lock", debug.Stack())
}

// TryLock tries to lock rw for writing and reports whether it succeeded.
//
// Note that while correct uses of TryLock do exist, they are rare,
// and use of TryLock is often a sign of a deeper problem
// in a particular use of mutexes.
func (m *RWMutex) TryLock() bool {
	locked := m.mutex.TryLock()

	if locked {
		m.unlocking = startLockTimer("RWMutex timed out before releasing lock", debug.Stack())
	}

	return locked
}

// Unlock unlocks rw for writing. It is a run-time error if rw is
// not locked for writing on entry to Unlock.
//
// As with Mutexes, a locked RWMutex is not associated with a particular
// goroutine. One goroutine may RLock (Lock) a RWMutex and then
// arrange for another goroutine to RUnlock (Unlock) it.
func (m *RWMutex) Unlock() {
	close(m.unlocking)
	m.mutex.Unlock()
}

// Happens-before relationships are indicated to the race detector via:
// - Unlock  -> Lock:  readerSem
// - Unlock  -> RLock: readerSem
// - RUnlock -> Lock:  writerSem
//
// The methods below temporarily disable handling of race synchronization
// events in order to provide the more precise model above to the race
// detector.
//
// For example, atomic.AddInt32 in RLock should not appear to provide
// acquire-release semantics, which would incorrectly synchronize racing
// readers, thus potentially missing races.

// RLock locks rw for reading.
//
// It should not be used for recursive read locking; a blocked Lock
// call excludes new readers from acquiring the lock. See the
// documentation on the RWMutex type.
func (m *RWMutex) RLock() {
	locking := startLockTimer("RWMutex timed out when acquiring RLock", debug.Stack())
	m.mutex.RLock()
	close(locking)

	m.startRLockTimer("RMutex timed out before releasing RLock", debug.Stack())
}

// TryRLock tries to lock rw for reading and reports whether it succeeded.
//
// Note that while correct uses of TryRLock do exist, they are rare,
// and use of TryRLock is often a sign of a deeper problem
// in a particular use of mutexes.
func (m *RWMutex) TryRLock() bool {
	locked := m.mutex.TryRLock()
	if locked {
		m.startRLockTimer("RMutex timed out before releasing RLock", debug.Stack())
	}
	return locked
}

// RUnlock undoes a single RLock call;
// it does not affect other simultaneous readers.
// It is a run-time error if rw is not locked for reading
// on entry to RUnlock.
func (m *RWMutex) RUnlock() {
	m.wg.Done()
	m.mutex.RUnlock()
}

func (m *RWMutex) startRLockTimer(msg string, stack []byte) {
	m.wg.Add(1)

	if m.wgStarted {
		return
	}

	m.wgStarted = true

	done := startLockTimer(msg, stack)
	go func() {
		m.wg.Wait()
		close(done)
	}()
}
