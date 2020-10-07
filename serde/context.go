//
// Documentation Last Review: 07.10.2020
//

package serde

// ContextEngine is the interface to implement to create a context.
type ContextEngine interface {
	// GetFormat returns the name of the format for this context.
	GetFormat() Format

	// Marshal returns the bytes of the message according to the format of the
	// context.
	Marshal(message interface{}) ([]byte, error)

	// Unmarshal populates the message with the data according to the format of
	// the context.
	Unmarshal(data []byte, message interface{}) error
}

// Context is the context passed to the serialization/deserialization requests.
type Context struct {
	ContextEngine

	factories map[interface{}]Factory
}

// NewContext returns a new empty context.
func NewContext(engine ContextEngine) Context {
	return Context{
		ContextEngine: engine,
		factories:     make(map[interface{}]Factory),
	}
}

// GetFactory returns the factory associated to the key or nil.
func (ctx Context) GetFactory(key interface{}) Factory {
	return ctx.factories[key]
}

// WithFactory adds a factory to the context. The factory will then be availble
// with the key when deserializing.
func WithFactory(ctx Context, key interface{}, f Factory) Context {
	factories := map[interface{}]Factory{}

	for key, value := range ctx.factories {
		factories[key] = value
	}

	factories[key] = f

	// Prevent parent context from being contaminated with the new factory.
	ctx.factories = factories

	return ctx
}
