package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	kitgrpc "github.com/go-kit/kit/transport/grpc"
	stdopentracing "github.com/opentracing/opentracing-go"
	"github.com/openzipkin/zipkin-go"
	stdzipkin "github.com/openzipkin/zipkin-go"
	zipkinhttp "github.com/openzipkin/zipkin-go/reporter/http"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	pb "github.com/cage1016/gokitconsulk8s/pb/foosvc"
	addsvctransports "github.com/cage1016/gokitconsulk8s/pkg/addsvc/transports"
	"github.com/cage1016/gokitconsulk8s/pkg/foosvc/endpoints"
	"github.com/cage1016/gokitconsulk8s/pkg/foosvc/service"
	"github.com/cage1016/gokitconsulk8s/pkg/foosvc/transports"
)

const (
	defZipkinV2URL string = ""
	defNameSpace   string = "gokitconsulk8s"
	defServiceName string = "foosvc"
	defLogLevel    string = "error"
	defServiceHost string = "localhost"
	defHTTPPort    string = "8180"
	defGRPCPort    string = "8181"
	defAddsvcURL   string = ""

	envZipkinV2URL string = "QS_ZIPKIN_V2_URL"
	envNameSpace   string = "QS_FOOSVC_NAMESPACE"
	envServiceName string = "QS_FOOSVC_SERVICE_NAME"
	envLogLevel    string = "QS_FOOSVC_LOG_LEVEL"
	envServiceHost string = "QS_FOOSVC_SERVICE_HOST"
	envHTTPPort    string = "QS_FOOSVC_HTTP_PORT"
	envGRPCPort    string = "QS_FOOSVC_GRPC_PORT"
	envAddsvcURL   string = "QS_ADDSVC_URL"
)

type config struct {
	nameSpace   string
	serviceName string
	logLevel    string
	serviceHost string
	httpPort    string
	grpcPort    string
	zipkinV2URL string
	addsvcURL   string
}

// Env reads specified environment variable. If no value has been found,
// fallback is returned.
func env(key string, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	var logger log.Logger
	{
		logger = log.NewLogfmtLogger(os.Stderr)
		logger = level.NewFilter(logger, level.AllowInfo())
		logger = log.With(logger, "ts", log.DefaultTimestampUTC)
		logger = log.With(logger, "caller", log.DefaultCaller)
	}
	cfg := loadConfig(logger)
	logger = log.With(logger, "service", cfg.serviceName)

	// addsvc grpc connection
	var conn *grpc.ClientConn
	{
		var err error
		if cfg.addsvcURL != "" {
			conn, err = grpc.Dial(cfg.addsvcURL, grpc.WithInsecure())
			if err != nil {
				level.Error(logger).Log("serviceName", cfg.addsvcURL, "error", err)
				os.Exit(1)
			}
		}
	}

	tracer := initOpentracing()
	zipkinTracer := initZipkin(cfg.serviceName, cfg.httpPort, cfg.zipkinV2URL, logger)

	service := NewServer(conn, tracer, zipkinTracer, logger)
	endpoints := endpoints.New(service, logger, tracer, zipkinTracer)

	errs := make(chan error, 2)
	hs := health.NewServer()
	hs.SetServingStatus(cfg.serviceName, healthgrpc.HealthCheckResponse_SERVING)
	go startHTTPServer(endpoints, tracer, zipkinTracer, cfg.httpPort, logger, errs)
	go startGRPCServer(endpoints, tracer, zipkinTracer, cfg.grpcPort, hs, logger, errs)

	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT)
		errs <- fmt.Errorf("%s", <-c)
	}()

	err := <-errs
	level.Info(logger).Log("serviceName", cfg.serviceName, "terminated", err)
}

func loadConfig(logger log.Logger) (cfg config) {
	cfg.nameSpace = env(envNameSpace, defNameSpace)
	cfg.serviceName = env(envServiceName, defServiceName)
	cfg.logLevel = env(envLogLevel, defLogLevel)
	cfg.serviceHost = env(envServiceHost, defServiceHost)
	cfg.httpPort = env(envHTTPPort, defHTTPPort)
	cfg.grpcPort = env(envGRPCPort, defGRPCPort)
	cfg.zipkinV2URL = env(envZipkinV2URL, defZipkinV2URL)
	cfg.addsvcURL = env(envAddsvcURL, defAddsvcURL)
	return cfg
}

func NewServer(conn *grpc.ClientConn, tracer stdopentracing.Tracer, zipkinTracer *stdzipkin.Tracer, logger log.Logger) (service.FoosvcService) {
	addsvcservice := addsvctransports.NewGRPCClient(conn, tracer, zipkinTracer, logger)
	service := service.New(addsvcservice, logger)
	return service
}

func initOpentracing() (tracer stdopentracing.Tracer) {
	return stdopentracing.GlobalTracer()
}

func initZipkin(serviceName, httpPort, zipkinV2URL string, logger log.Logger) (zipkinTracer *zipkin.Tracer) {
	var (
		err           error
		hostPort      = fmt.Sprintf("localhost:%s", httpPort)
		useNoopTracer = (zipkinV2URL == "")
		reporter      = zipkinhttp.NewReporter(zipkinV2URL)
	)
	zEP, _ := zipkin.NewEndpoint(serviceName, hostPort)
	zipkinTracer, err = zipkin.NewTracer(reporter, zipkin.WithLocalEndpoint(zEP), zipkin.WithNoopTracer(useNoopTracer))
	if err != nil {
		logger.Log("err", err)
		os.Exit(1)
	}
	if !useNoopTracer {
		logger.Log("tracer", "Zipkin", "type", "Native", "URL", zipkinV2URL)
	}

	return
}

func startHTTPServer(endpoints endpoints.Endpoints, tracer stdopentracing.Tracer, zipkinTracer *zipkin.Tracer, port string, logger log.Logger, errs chan error) {
	p := fmt.Sprintf(":%s", port)
	level.Info(logger).Log("protocol", "HTTP", "exposed", port)
	errs <- http.ListenAndServe(p, transports.NewHTTPHandler(endpoints, tracer, zipkinTracer, logger))
}

func startGRPCServer(endpoints endpoints.Endpoints, tracer stdopentracing.Tracer, zipkinTracer *zipkin.Tracer, port string, hs *health.Server, logger log.Logger, errs chan error) {
	p := fmt.Sprintf(":%s", port)
	listener, err := net.Listen("tcp", p)
	if err != nil {
		level.Error(logger).Log("protocol", "GRPC", "listen", port, "err", err)
		os.Exit(1)
	}

	var server *grpc.Server
	level.Info(logger).Log("protocol", "GRPC", "protocol", "GRPC", "exposed", port)
	server = grpc.NewServer(grpc.UnaryInterceptor(kitgrpc.Interceptor))
	pb.RegisterFoosvcServer(server, transports.MakeGRPCServer(endpoints, tracer, zipkinTracer, logger))
	healthgrpc.RegisterHealthServer(server, hs)
	reflection.Register(server)
	errs <- server.Serve(listener)
}
