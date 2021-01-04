package tracing

import (
	"io"
	"sync"

	opentracing "github.com/opentracing/opentracing-go"
	_ "github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
)

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

	tracer, closer, err := cfg.New(addr)
	if err != nil {
		return nil, err
	}
	tracerInstance.addrTracer[addr] = tracerCloser{
		tracer: tracer,
		closer: closer,
	}
	return tracer, nil
}

func CloseAll() error {
	for _, tc := range tracerInstance.addrTracer {
		err := tc.closer.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
