package fake

import opentracing "github.com/opentracing/opentracing-go"

// GetTracerForAddrWithError is used to mock `tracing.GetTracerForAddr` with an
// error.
func GetTracerForAddrWithError(addr string) (opentracing.Tracer, error) {
	return nil, fakeErr
}

// GetTracerForAddrEmpty is used to mock `tracing.GetTracerForAddr` with an
// empty tracer.
func GetTracerForAddrEmpty(_ string) (opentracing.Tracer, error) {
	return opentracing.NoopTracer{}, nil
}
