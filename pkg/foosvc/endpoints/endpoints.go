package endpoints

import (
	"context"
	"time"

	"github.com/cage1016/gokitsonsulk8s/pkg/foosvc/service"
	"github.com/go-kit/kit/circuitbreaker"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/ratelimit"
	"github.com/go-kit/kit/tracing/opentracing"
	"github.com/go-kit/kit/tracing/zipkin"
	stdopentracing "github.com/opentracing/opentracing-go"
	stdzipkin "github.com/openzipkin/zipkin-go"
	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"
)

// Endpoints collects all of the endpoints that compose the foosvc service. It's
// meant to be used as a helper struct, to collect all of the endpoints into a
// single parameter.
type Endpoints struct {
	FooEndpoint endpoint.Endpoint `json:""`
}

// New return a new instance of the endpoint that wraps the provided service.
func New(svc service.FoosvcService, logger log.Logger, otTracer stdopentracing.Tracer, zipkinTracer *stdzipkin.Tracer) (ep Endpoints) {
	var fooEndpoint endpoint.Endpoint
	{
		method := "foo"
		fooEndpoint = MakeFooEndpoint(svc)
		fooEndpoint = ratelimit.NewErroringLimiter(rate.NewLimiter(rate.Every(time.Second), 100))(fooEndpoint)
		fooEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{}))(fooEndpoint)
		fooEndpoint = opentracing.TraceServer(otTracer, method)(fooEndpoint)
		fooEndpoint = zipkin.TraceEndpoint(zipkinTracer, method)(fooEndpoint)
		fooEndpoint = LoggingMiddleware(log.With(logger, "method", method))(fooEndpoint)
		ep.FooEndpoint = fooEndpoint
	}

	return ep
}

// MakeFooEndpoint returns an endpoint that invokes Foo on the service.
// Primarily useful in a server.
func MakeFooEndpoint(svc service.FoosvcService) (ep endpoint.Endpoint) {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(FooRequest)
		if err := req.validate(); err != nil {
			return FooResponse{}, err
		}
		res, err := svc.Foo(ctx, req.S)
		return FooResponse{Res: res}, err
	}
}

// Foo implements the service interface, so Endpoints may be used as a service.
// This is primarily useful in the context of a client library.
func (e Endpoints) Foo(ctx context.Context, s string) (res string, err error) {
	resp, err := e.FooEndpoint(ctx, FooRequest{S: s})
	if err != nil {
		return
	}
	response := resp.(FooResponse)
	return response.Res, nil
}
