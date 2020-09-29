package pow

import "go.dedis.ch/dela/core/ordering"

type observer struct {
	events chan ordering.Event
}

func (o observer) NotifyCallback(event interface{}) {
	o.events <- event.(ordering.Event)
}
