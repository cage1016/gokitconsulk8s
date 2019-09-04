package transport

import (
	"context"
	"net/http"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
	stdopentracing "github.com/opentracing/opentracing-go"
	stdzipkin "github.com/openzipkin/zipkin-go"
	"google.golang.org/grpc"

	addsvcendpoint "github.com/cage1016/gokitsonsulk8s/pkg/addsvc/endpoints"
	addsvcservice "github.com/cage1016/gokitsonsulk8s/pkg/addsvc/service"
	addsvctransports "github.com/cage1016/gokitsonsulk8s/pkg/addsvc/transports"
	foosvcendpoint "github.com/cage1016/gokitsonsulk8s/pkg/foosvc/endpoints"
	foosvcservice "github.com/cage1016/gokitsonsulk8s/pkg/foosvc/service"
	foosvctransports "github.com/cage1016/gokitsonsulk8s/pkg/foosvc/transports"
)

func MakeHandler(ctx context.Context, addsvc, foosvc string, retryMax, retryTimeout int64, tracer stdopentracing.Tracer, zipkinTracer *stdzipkin.Tracer, logger log.Logger) http.Handler {
	r := mux.NewRouter()

	// addsvc
	{
		var (
			endpoints = addsvcendpoint.Endpoints{}
		)
		{
			factory, _ := addSvcFactory(ctx, addsvc, addsvcendpoint.MakeSumEndpoint, tracer, zipkinTracer, logger)
			endpoints.SumEndpoint = factory
		}
		{
			factory, _ := addSvcFactory(ctx, addsvc, addsvcendpoint.MakeConcatEndpoint, tracer, zipkinTracer, logger)
			endpoints.ConcatEndpoint = factory
		}
		r.PathPrefix("/addsvc").Handler(http.StripPrefix("/addsvc", addsvctransports.NewHTTPHandler(endpoints, tracer, zipkinTracer, logger)))
	}

	// foo
	{
		var (
			endpoints = foosvcendpoint.Endpoints{}
		)
		{
			factory, _ := fooSvcFactory(ctx, foosvc, foosvcendpoint.MakeFooEndpoint, tracer, zipkinTracer, logger)
			endpoints.FooEndpoint = factory
		}
		r.PathPrefix("/foosvc").Handler(http.StripPrefix("/foosvc", foosvctransports.NewHTTPHandler(endpoints, tracer, zipkinTracer, logger)))
	}

	r.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("ok"))
	})

	return r
}

func addSvcFactory(
	ctx context.Context,
	addsvc string,
	makeEndpoint func(addsvcservice.AddsvcService) endpoint.Endpoint,
	tracer stdopentracing.Tracer,
	zipkinTracer *stdzipkin.Tracer,
	logger log.Logger) (endpoint.Endpoint, error) {
	// We could just as easily use the HTTP or Thrift client package to make
	// the connection to addsvc. We've chosen gRPC arbitrarily. Note that
	// the transport is an implementation detail: it doesn't leak out of
	// this function. Nice!

	conn, err := grpc.Dial(addsvc, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	service := addsvctransports.NewGRPCClient(conn, tracer, zipkinTracer, logger)

	return makeEndpoint(service), nil
}

func fooSvcFactory(
	ctx context.Context,
	foosvc string,
	makeEndpoint func(foosvcservice.FoosvcService) endpoint.Endpoint,
	tracer stdopentracing.Tracer,
	zipkinTracer *stdzipkin.Tracer,
	logger log.Logger) (endpoint.Endpoint, error) {
	// We could just as easily use the HTTP or Thrift client package to make
	// the connection to foosvc. We've chosen gRPC arbitrarily. Note that
	// the transport is an implementation detail: it doesn't leak out of
	// this function. Nice!

	conn, err := grpc.Dial(foosvc, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	service := foosvctransports.NewGRPCClient(conn, tracer, zipkinTracer, logger)

	// Notice that the addsvc gRPC client converts the connection to a
	// complete addsvc, and we just throw away everything except the method
	// we're interested in. A smarter factory would mux multiple methods
	// over the same connection. But that would require more work to manage
	// the returned io.Closer, e.g. reference counting. Since this is for
	// the purposes of demonstration, we'll just keep it simple.

	return makeEndpoint(service), nil
}
