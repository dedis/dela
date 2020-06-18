package serdeng

type ContextEngine interface {
	GetName() Codec
	Marshal(interface{}) ([]byte, error)
	Unmarshal([]byte, interface{}) error
}

type Context struct {
	ContextEngine

	factories map[interface{}]Factory
}

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

func NewContext(engine ContextEngine) Context {
	return Context{
		ContextEngine: engine,
		factories:     make(map[interface{}]Factory),
	}
}

func (ctx Context) GetFactory(key interface{}) Factory {
	return ctx.factories[key]
}
