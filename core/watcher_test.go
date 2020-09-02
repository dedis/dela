package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWatcher_Add(t *testing.T) {
	watcher := NewWatcher()

	watcher.Add(fakeObserver{ch: make(chan interface{})})
	require.Len(t, watcher.observers, 1)

	obs := fakeObserver{ch: make(chan interface{})}
	watcher.Add(obs)
	require.Len(t, watcher.observers, 2)

	watcher.Add(obs)
	require.Len(t, watcher.observers, 2)
}

func TestWatcher_Remove(t *testing.T) {
	watcher := NewWatcher()
	watcher.observers[newFakeObserver()] = struct{}{}

	obs := newFakeObserver()
	watcher.observers[obs] = struct{}{}
	require.Len(t, watcher.observers, 2)

	watcher.Remove(obs)
	require.Len(t, watcher.observers, 1)

	watcher.Remove(obs)
	require.Len(t, watcher.observers, 1)
}

func TestWatcher_Notify(t *testing.T) {
	watcher := NewWatcher()

	obs := newFakeObserver()
	watcher.observers[obs] = struct{}{}

	watcher.Notify(struct{}{})
	evt := <-obs.ch
	require.NotNil(t, evt)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeObserver struct {
	ch chan interface{}
}

func (o fakeObserver) NotifyCallback(evt interface{}) {
	o.ch <- evt
}

func newFakeObserver() fakeObserver {
	return fakeObserver{
		ch: make(chan interface{}, 1),
	}
}
