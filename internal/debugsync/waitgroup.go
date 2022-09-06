package debugsync

import (
	"fmt"
	"runtime/debug"
	"sync"
)

type WaitGroup struct {
	wg sync.WaitGroup
}

// Add adds delta, which may be negative, to the WaitGroup counter.
// If the counter becomes zero, all goroutines blocked on Wait are released.
// If the counter goes negative, Add panics.
//
// Note that calls with a positive delta that occur when the counter is zero
// must happen before a Wait. Calls with a negative delta, or calls with a
// positive delta that start when the counter is greater than zero, may happen
// at any time.
// Typically this means the calls to Add should execute before the statement
// creating the goroutine or other event to be waited for.
// If a WaitGroup is reused to wait for several independent sets of events,
// new Add calls must happen after all previous Wait calls have returned.
// See the WaitGroup example.
func (wg *WaitGroup) Add(delta int) {
	wg.wg.Add(delta)
}

// Done decrements the WaitGroup counter by one.
func (wg *WaitGroup) Done() {
	wg.wg.Done()
}

// Wait blocks until the WaitGroup counter is zero.
func (wg *WaitGroup) Wait() {
	msg := fmt.Sprintf("WaitGroup %v timed out", wg.wg)
	waiting := startLockTimer(msg, debug.Stack())
	wg.wg.Wait()
	close(waiting)
}
