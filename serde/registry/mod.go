// Package registry defines the format registry mechanism.
//
// It also provides a default implementation that will always return a format
// using an empty format when appropriate. This format will always return an
// error.
//
// Documentation Last Review: 07.10.2020
//
package registry

import (
	"go.dedis.ch/dela/serde"
)

// Registry is an interface to register and get format engines for a specific
// format.
type Registry interface {
	// Register takes a format and its engine and it registers them so that the
	// engine can be looked up later.
	Register(serde.Format, serde.FormatEngine)

	// Get returns the engine associated with the format.
	Get(serde.Format) serde.FormatEngine
}
