package transports

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-kit/kit/circuitbreaker"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/ratelimit"
	"github.com/go-kit/kit/sd/lb"
	"github.com/go-kit/kit/tracing/opentracing"
	"github.com/go-kit/kit/tracing/zipkin"
	httptransport "github.com/go-kit/kit/transport/http"
	stdopentracing "github.com/opentracing/opentracing-go"
	stdzipkin "github.com/openzipkin/zipkin-go"
	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"
	"google.golang.org/grpc/status"

	"github.com/cage1016/gokitsonsulk8s/pkg/foosvc/endpoints"
	"github.com/cage1016/gokitsonsulk8s/pkg/foosvc/service"
)

type errorWrapper struct {
	Error string `json:"error"`
}

func JSONErrorDecoder(r *http.Response) error {
	contentType := r.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return fmt.Errorf("expected JSON formatted error, got Content-Type %s", contentType)
	}
	var w errorWrapper
	if err := json.NewDecoder(r.Body).Decode(&w); err != nil {
		return err
	}
	return errors.New(w.Error)
}

// NewHTTPHandler returns a handler that makes a set of endpoints available on
// predefined paths.
func NewHTTPHandler(endpoints endpoints.Endpoints, otTracer stdopentracing.Tracer, zipkinTracer *stdzipkin.Tracer, logger log.Logger) http.Handler { // Zipkin HTTP Server Trace can either be instantiated per endpoint with a
	// provided operation name or a global tracing service can be instantiated
	// without an operation name and fed to each Go kit endpoint as ServerOption.
	// In the latter case, the operation name will be the endpoint's http method.
	// We demonstrate a global tracing service here.
	zipkinServer := zipkin.HTTPServerTrace(zipkinTracer)

	options := []httptransport.ServerOption{
		httptransport.ServerErrorEncoder(httpEncodeError),
		httptransport.ServerErrorLogger(logger),
		zipkinServer,
	}

	m := http.NewServeMux()
	m.Handle("/foo", httptransport.NewServer(
		endpoints.FooEndpoint,
		decodeHTTPFooRequest,
		httptransport.EncodeJSONResponse,
		append(options, httptransport.ServerBefore(opentracing.HTTPToContext(otTracer, "Foo", logger)))...,
	))
	return m
}

// decodeHTTPFooRequest is a transport/http.DecodeRequestFunc that decodes a
// JSON-encoded request from the HTTP request body. Primarily useful in a server.
func decodeHTTPFooRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req endpoints.FooRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}

// NewHTTPClient returns an AddService backed by an HTTP server living at the
// remote instance. We expect instance to come from a service discovery system,
// so likely of the form "host:port". We bake-in certain middlewares,
// implementing the client library pattern.
func NewHTTPClient(instance string, otTracer stdopentracing.Tracer, zipkinTracer *stdzipkin.Tracer, logger log.Logger) (service.FoosvcService, error) { // Quickly sanitize the instance string.
	if !strings.HasPrefix(instance, "http") {
		instance = "http://" + instance
	}
	u, err := url.Parse(instance)
	if err != nil {
		return nil, err
	}

	// We construct a single ratelimiter middleware, to limit the total outgoing
	// QPS from this client to all methods on the remote instance. We also
	// construct per-endpoint circuitbreaker middlewares to demonstrate how
	// that's done, although they could easily be combined into a single breaker
	// for the entire remote instance, too.
	limiter := ratelimit.NewErroringLimiter(rate.NewLimiter(rate.Every(time.Second), 100))

	// Zipkin HTTP Client Trace can either be instantiated per endpoint with a
	// provided operation name or a global tracing client can be instantiated
	// without an operation name and fed to each Go kit endpoint as ClientOption.
	// In the latter case, the operation name will be the endpoint's http method.
	zipkinClient := zipkin.HTTPClientTrace(zipkinTracer)

	// global client middlewares
	options := []httptransport.ClientOption{
		zipkinClient,
	}

	e := endpoints.Endpoints{}

	// Each individual endpoint is an http/transport.Client (which implements
	// endpoint.Endpoint) that gets wrapped with various middlewares. If you
	// made your own client library, you'd do this work there, so your server
	// could rely on a consistent set of client behavior.
	// The Foo endpoint is the same thing, with slightly different
	// middlewares to demonstrate how to specialize per-endpoint.
	var fooEndpoint endpoint.Endpoint
	{
		fooEndpoint = httptransport.NewClient(
			"POST",
			copyURL(u, "/foo"),
			encodeHTTPFooRequest,
			decodeHTTPFooResponse,
			append(options, httptransport.ClientBefore(opentracing.ContextToHTTP(otTracer, logger)))...,
		).Endpoint()
		fooEndpoint = opentracing.TraceClient(otTracer, "Foo")(fooEndpoint)
		fooEndpoint = zipkin.TraceEndpoint(zipkinTracer, "Foo")(fooEndpoint)
		fooEndpoint = limiter(fooEndpoint)
		fooEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:    "Foo",
			Timeout: 30 * time.Second,
		}))(fooEndpoint)
		e.FooEndpoint = fooEndpoint
	}

	// Returning the endpoint.Set as a service.Service relies on the
	// endpoint.Set implementing the Service methods. That's just a simple bit
	// of glue code.
	return e, nil
}

//
func copyURL(base *url.URL, path string) *url.URL {
	next := *base
	next.Path = path
	return &next
}

// encodeHTTPFooRequest is a transport/http.EncodeRequestFunc that
// JSON-encodes any request to the request body. Primarily useful in a client.
func encodeHTTPFooRequest(_ context.Context, r *http.Request, request interface{}) (err error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(request); err != nil {
		return err
	}
	r.Body = ioutil.NopCloser(&buf)
	return nil
}

// decodeHTTPFooResponse is a transport/http.DecodeResponseFunc that decodes a
// JSON-encoded sum response from the HTTP response body. If the response has a
// non-200 status code, we will interpret that as an error and attempt to decode
// the specific error message from the response body. Primarily useful in a client.
func decodeHTTPFooResponse(_ context.Context, r *http.Response) (interface{}, error) {
	if r.StatusCode != http.StatusOK {
		return nil, JSONErrorDecoder(r)
	}
	var resp endpoints.FooResponse
	err := json.NewDecoder(r.Body).Decode(&resp)
	return resp, err
}

func httpEncodeError(_ context.Context, err error, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")

	if lberr, ok := err.(lb.RetryError); ok {
		st, _ := status.FromError(lberr.Final)
		w.WriteHeader(HTTPStatusFromCode(st.Code()))
		json.NewEncoder(w).Encode(errorWrapper{Error: st.Message()})
	} else {
		st, ok := status.FromError(err)
		if ok {
			w.WriteHeader(HTTPStatusFromCode(st.Code()))
			json.NewEncoder(w).Encode(errorWrapper{Error: st.Message()})
		} else {
			switch err {
			case io.ErrUnexpectedEOF:
				w.WriteHeader(http.StatusBadRequest)
			case io.EOF:
				w.WriteHeader(http.StatusBadRequest)
			default:
				switch err.(type) {
				case *json.SyntaxError:
					w.WriteHeader(http.StatusBadRequest)
				case *json.UnmarshalTypeError:
					w.WriteHeader(http.StatusBadRequest)
				default:
					w.WriteHeader(http.StatusInternalServerError)
				}
			}
			json.NewEncoder(w).Encode(errorWrapper{Error: err.Error()})
		}
	}
}
