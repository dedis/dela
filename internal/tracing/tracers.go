package tracing

import (
	"io"
	"sync"

	opentracing "github.com/opentracing/opentracing-go"
	_ "github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	"golang.org/x/xerrors"
)

type key int

// ProtocolKey is the key used to denote a protocol in a `context.Context`.
const ProtocolKey key = iota

var (
	// ProtocolTag specified the span tag used for denoting a protocol.
	ProtocolTag = "protocol"
	// UndefinedProtocol is the default ProtocolTag value used if no
	// ProtocolKey is present in the context.
	UndefinedProtocol = "__UNDEFINED_PROTOCOL__"
)

type tracerCatalog struct {
	tracerByAddr map[string]closableTracer
	sync.Mutex
}

type closableTracer struct {
	tracer opentracing.Tracer
	closer io.Closer
}

var catalog = tracerCatalog{
	tracerByAddr: make(map[string]closableTracer),
}

// GetTracerForAddr returns an `opentracing.Tracer` instance for the given
// address. Since the tracers are cached, it returns an existing one if it
// has been initialized before.
func GetTracerForAddr(addr string) (opentracing.Tracer, error) {
	catalog.Lock()
	defer catalog.Unlock()

	tc, ok := catalog.tracerByAddr[addr]
	if ok {
		return tc.tracer, nil
	}

	cfg, err := jaegercfg.FromEnv()
	if err != nil {
		return nil, xerrors.Errorf("error parsing jaeger configuration from environment: %v", err)
	}

	cfg.ServiceName = addr
	tracer, closer, err := cfg.NewTracer()
	if err != nil {
		return nil, xerrors.Errorf("error creating new tracer: %v", err)
	}

	catalog.tracerByAddr[addr] = closableTracer{
		tracer: tracer,
		closer: closer,
	}

	return tracer, nil
}

// CloseAll closes all the tracer instances.
func CloseAll() error {
	for _, tc := range catalog.tracerByAddr {
		err := tc.closer.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
