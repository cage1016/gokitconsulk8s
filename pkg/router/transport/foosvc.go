package transport

import (
	"context"
	"net/http"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"

	stdopentracing "github.com/opentracing/opentracing-go"
	stdzipkin "github.com/openzipkin/zipkin-go"
	"google.golang.org/grpc"

	"github.com/cage1016/gokitconsulk8s/pkg/foosvc/endpoints"
	"github.com/cage1016/gokitconsulk8s/pkg/foosvc/service"
	"github.com/cage1016/gokitconsulk8s/pkg/foosvc/transports"
)

func MakeFooSvcHandler(ctx context.Context, target string, tracer stdopentracing.Tracer, zipkinTracer *stdzipkin.Tracer, logger log.Logger) http.Handler {
	var eps = endpoints.Endpoints{}
	eps.FooEndpoint = fooSvcFactory(ctx, target, endpoints.MakeFooEndpoint, tracer, zipkinTracer, logger)

	return transports.NewHTTPHandler(eps, tracer, zipkinTracer, logger)
}

func fooSvcFactory(
	ctx context.Context,
	target string,
	makeEndpoint func(service.FoosvcService) endpoint.Endpoint,
	tracer stdopentracing.Tracer,
	zipkinTracer *stdzipkin.Tracer,
	logger log.Logger) (endpoint.Endpoint) {

	conn, err := grpc.DialContext(ctx, target, grpc.WithInsecure())
	if err != nil {
		return func(ctx context.Context, request interface{}) (interface{}, error) {
			return nil, err
		}
	}
	svc := transports.NewGRPCClient(conn, tracer, zipkinTracer, logger)

	return makeEndpoint(svc)
}
