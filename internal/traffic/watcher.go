package traffic

import (
	"context"

	"go.dedis.ch/dela/core"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
)

// Watcher describes a Watcher of outgoing and incoming events.
type Watcher interface {
	WatchOuts(ctx context.Context) <-chan Event
	WatchIns(ctx context.Context) <-chan Event
}

// Notifier describes the function to push events to a Watcher.
type Notifier interface {
	NotifyIn(mino.Address, router.Packet)
	NotifyOut(mino.Address, router.Packet)
}

// EventsHandler describes the functions to handle events
type EventsHandler interface {
	Watcher
	Notifier
}

// NewEventHandler returns a new initialized event handler.
func NewEventHandler() EventsHandler {
	return &DefaultEventHandlers{
		outWatcher: core.NewWatcher(),
		inWatcher:  core.NewWatcher(),
	}
}

// DefaultEventHandlers provides a default implementation of an EventHandler.
//
// - implements EventsHandler
type DefaultEventHandlers struct {
	outWatcher core.Observable
	inWatcher  core.Observable
}

// WatchOuts implements Watcher. It returns a channel populated with sent
// messages.
func (d *DefaultEventHandlers) WatchOuts(ctx context.Context) <-chan Event {
	return watch(ctx, d.outWatcher)
}

// WatchIns implements Watcher. It returns a channel populated with received
// messages.
func (d *DefaultEventHandlers) WatchIns(ctx context.Context) <-chan Event {
	return watch(ctx, d.inWatcher)
}

// NotifyIn implements Notifier.
func (d *DefaultEventHandlers) NotifyIn(a mino.Address, p router.Packet) {
	d.inWatcher.Notify(Event{Address: a, Pkt: p})
}

// NotifyOut implements Notifier.
func (d *DefaultEventHandlers) NotifyOut(a mino.Address, p router.Packet) {
	d.outWatcher.Notify(Event{Address: a, Pkt: p})
}

// watch is a generic function to watch for events
func watch(ctx context.Context, watcher core.Observable) <-chan Event {
	obs := observer{ch: make(chan Event, watcherSize)}

	watcher.Add(obs)

	go func() {
		<-ctx.Done()
		watcher.Remove(obs)
		close(obs.ch)
	}()

	return obs.ch
}
