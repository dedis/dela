package tracing

import (
	"io"
	"sync"

	opentracing "github.com/opentracing/opentracing-go"
	_ "github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
)

var (
	ProtocolTag = "protocol"
)

type key int

const ProtocolKey key = iota

type tracer struct {
	addrTracer map[string]tracerCloser
	sync.Mutex
}

type tracerCloser struct {
	tracer opentracing.Tracer
	closer io.Closer
}

var tracerInstance tracer

func init() {
	tracerInstance = tracer{
		addrTracer: make(map[string]tracerCloser),
	}
}

// GetTracerForAddr returns an `opentracing.Tracer` instance for the given
// address. Since the tracers are cached, it returns an existing one if it
// has been initialized before.
func GetTracerForAddr(addr string) (opentracing.Tracer, error) {
	tracerInstance.Lock()
	defer tracerInstance.Unlock()

	tc, ok := tracerInstance.addrTracer[addr]
	if ok {
		return tc.tracer, nil
	}

	cfg, err := jaegercfg.FromEnv()
	if err != nil {
		return nil, err
	}

	cfg.ServiceName = addr
	tracer, closer, err := cfg.NewTracer()
	if err != nil {
		return nil, err
	}
	tracerInstance.addrTracer[addr] = tracerCloser{
		tracer: tracer,
		closer: closer,
	}
	return tracer, nil
}

// CloseAll closes all the tracer instances.
func CloseAll() error {
	for _, tc := range tracerInstance.addrTracer {
		err := tc.closer.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
