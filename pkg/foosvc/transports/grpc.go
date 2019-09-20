package transports

import (
	"context"
	"time"

	"github.com/go-kit/kit/circuitbreaker"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/ratelimit"
	"github.com/go-kit/kit/tracing/opentracing"
	"github.com/go-kit/kit/tracing/zipkin"
	grpctransport "github.com/go-kit/kit/transport/grpc"
	stdopentracing "github.com/opentracing/opentracing-go"
	stdzipkin "github.com/openzipkin/zipkin-go"
	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/cage1016/gokitconsulk8s/pb/foosvc"
	"github.com/cage1016/gokitconsulk8s/pkg/foosvc/endpoints"
	"github.com/cage1016/gokitconsulk8s/pkg/foosvc/service"
)

type grpcServer struct {
	foo grpctransport.Handler `json:""`
}

func (s *grpcServer) Foo(ctx context.Context, req *pb.FooRequest) (rep *pb.FooReply, err error) {
	_, rp, err := s.foo.ServeGRPC(ctx, req)
	if err != nil {
		return nil, grpcEncodeError(err)
	}
	rep = rp.(*pb.FooReply)
	return rep, nil
}

// MakeGRPCServer makes a set of endpoints available as a gRPC server.
func MakeGRPCServer(endpoints endpoints.Endpoints, otTracer stdopentracing.Tracer, zipkinTracer *stdzipkin.Tracer, logger log.Logger) (req pb.FoosvcServer) { // Zipkin GRPC Server Trace can either be instantiated per gRPC method with a
	// provided operation name or a global tracing service can be instantiated
	// without an operation name and fed to each Go kit gRPC server as a
	// ServerOption.
	// In the latter case, the operation name will be the endpoint's grpc method
	// path if used in combination with the Go kit gRPC Interceptor.
	//
	// In this example, we demonstrate a global Zipkin tracing service with
	// Go kit gRPC Interceptor.
	zipkinServer := zipkin.GRPCServerTrace(zipkinTracer)

	options := []grpctransport.ServerOption{
		grpctransport.ServerErrorLogger(logger),
		zipkinServer,
	}

	return &grpcServer{
		foo: grpctransport.NewServer(
			endpoints.FooEndpoint,
			decodeGRPCFooRequest,
			encodeGRPCFooResponse,
			append(options, grpctransport.ServerBefore(opentracing.GRPCToContext(otTracer, "Foo", logger)))...,
		),
	}
}

// decodeGRPCFooRequest is a transport/grpc.DecodeRequestFunc that converts a
// gRPC request to a user-domain request. Primarily useful in a server.
func decodeGRPCFooRequest(_ context.Context, grpcReq interface{}) (interface{}, error) {
	req := grpcReq.(*pb.FooRequest)
	return endpoints.FooRequest{S: req.S}, nil
}

// encodeGRPCFooResponse is a transport/grpc.EncodeResponseFunc that converts a
// user-domain response to a gRPC reply. Primarily useful in a server.
func encodeGRPCFooResponse(_ context.Context, grpcReply interface{}) (res interface{}, err error) {
	reply := grpcReply.(endpoints.FooResponse)
	return &pb.FooReply{Res: reply.Res}, grpcEncodeError(reply.Err)
}

// NewGRPCClient returns an AddService backed by a gRPC server at the other end
// of the conn. The caller is responsible for constructing the conn, and
// eventually closing the underlying transport. We bake-in certain middlewares,
// implementing the client library pattern.
func NewGRPCClient(conn *grpc.ClientConn, otTracer stdopentracing.Tracer, zipkinTracer *stdzipkin.Tracer, logger log.Logger) service.FoosvcService { // We construct a single ratelimiter middleware, to limit the total outgoing
	// QPS from this client to all methods on the remote instance. We also
	// construct per-endpoint circuitbreaker middlewares to demonstrate how
	// that's done, although they could easily be combined into a single breaker
	// for the entire remote instance, too.
	limiter := ratelimit.NewErroringLimiter(rate.NewLimiter(rate.Every(time.Second), 100))

	// Zipkin GRPC Client Trace can either be instantiated per gRPC method with a
	// provided operation name or a global tracing client can be instantiated
	// without an operation name and fed to each Go kit client as ClientOption.
	// In the latter case, the operation name will be the endpoint's grpc method
	// path.
	//
	// In this example, we demonstrace a global tracing client.
	zipkinClient := zipkin.GRPCClientTrace(zipkinTracer)

	// global client middlewares
	options := []grpctransport.ClientOption{
		zipkinClient,
	}

	// The Foo endpoint is the same thing, with slightly different
	// middlewares to demonstrate how to specialize per-endpoint.
	var fooEndpoint endpoint.Endpoint
	{
		fooEndpoint = grpctransport.NewClient(
			conn,
			"pb.Foosvc",
			"Foo",
			encodeGRPCFooRequest,
			decodeGRPCFooResponse,
			pb.FooReply{},
			append(options, grpctransport.ClientBefore(opentracing.ContextToGRPC(otTracer, logger)))...,
		).Endpoint()
		fooEndpoint = opentracing.TraceClient(otTracer, "Foo")(fooEndpoint)
		fooEndpoint = limiter(fooEndpoint)
		fooEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:    "Foo",
			Timeout: 30 * time.Second,
		}))(fooEndpoint)
	}

	return endpoints.Endpoints{
		FooEndpoint: fooEndpoint,
	}
}

// encodeGRPCFooRequest is a transport/grpc.EncodeRequestFunc that converts a
// user-domain Foo request to a gRPC Foo request. Primarily useful in a client.
func encodeGRPCFooRequest(_ context.Context, request interface{}) (interface{}, error) {
	req := request.(endpoints.FooRequest)
	return &pb.FooRequest{S: req.S}, nil
}

// decodeGRPCFooResponse is a transport/grpc.DecodeResponseFunc that converts a
// gRPC Foo reply to a user-domain Foo response. Primarily useful in a client.
func decodeGRPCFooResponse(_ context.Context, grpcReply interface{}) (interface{}, error) {
	reply := grpcReply.(*pb.FooReply)
	return endpoints.FooResponse{Res: reply.Res}, nil
}

func grpcEncodeError(err error) error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if ok {
		return status.Error(st.Code(), st.Message())
	}
	switch err {
	default:
		return status.Error(codes.Internal, "internal server error")
	}
}
