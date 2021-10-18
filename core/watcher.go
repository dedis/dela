// Package core implements commonly used tools.
//
// Documentation Last Review: 08.10.2020
//
package core

import "sync"

// Observer is the interface to implement to watch events.
type Observer interface {
	NotifyCallback(event interface{})
}

// Observable provides primitives to add and remove observers and to notify
// them of new events.
type Observable interface {
	// Add adds the observer to the list of observers that will be notified of
	// new events.
	Add(observer Observer)

	// Remove removes the observer from the list thus stopping it from receiving
	// new events.
	Remove(observer Observer)

	// Notify notifies the observers of a new event.
	Notify(event interface{})
}

// Watcher is an implementation of the Observable interface.
//
// - implements core.Observable
type Watcher struct {
	sync.RWMutex

	observers map[Observer]struct{}
}

// NewWatcher creates a new empty watcher.
func NewWatcher() *Watcher {
	return &Watcher{
		observers: make(map[Observer]struct{}),
	}
}

// Add implements core.Observable. It adds the observer to the list of observers
// that will be notified of new events.
func (w *Watcher) Add(observer Observer) {
	w.Lock()
	w.observers[observer] = struct{}{}
	w.Unlock()
}

// Remove implements core.Observable. It removes the observer from the list thus
// stopping it from receiving new events.
func (w *Watcher) Remove(observer Observer) {
	w.Lock()
	delete(w.observers, observer)
	w.Unlock()
}

// Notify implements core.Observable. It notifies the whole list of observers
// one after each other.
func (w *Watcher) Notify(event interface{}) {
	w.RLock()
	defer w.RUnlock()

	for w := range w.observers {
		w.NotifyCallback(event)
	}
}
