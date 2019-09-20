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
	zipkinhttp "github.com/openzipkin/zipkin-go/reporter/http"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	pb "github.com/cage1016/gokitconsulk8s/pb/addsvc"
	"github.com/cage1016/gokitconsulk8s/pkg/addsvc/endpoints"
	"github.com/cage1016/gokitconsulk8s/pkg/addsvc/service"
	"github.com/cage1016/gokitconsulk8s/pkg/addsvc/transports"
)

const (
	defZipkinV2URL string = ""
	defNameSpace   string = "gokitconsulk8s"
	defServiceName string = "addsvc"
	defLogLevel    string = "error"
	defServiceHost string = "localhost"
	defHTTPPort    string = "8180"
	defGRPCPort    string = "8181"
	envZipkinV2URL string = "QS_ZIPKIN_V2_URL"
	envNameSpace   string = "QS_ADDSVC_NAMESPACE"
	envServiceName string = "QS_ADDSVC_SERVICE_NAME"
	envLogLevel    string = "QS_ADDSVC_LOG_LEVEL"
	envServiceHost string = "QS_ADDSVC_SERVICE_HOST"
	envHTTPPort    string = "QS_ADDSVC_HTTP_PORT"
	envGRPCPort    string = "QS_ADDSVC_GRPC_PORT"
)

type config struct {
	nameSpace   string
	serviceName string
	logLevel    string
	serviceHost string
	httpPort    string
	grpcPort    string
	zipkinV2URL string
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

	errs := make(chan error, 2)
	grpcServer, httpHandler := NewServer(cfg, logger)
	hs := health.NewServer()
	hs.SetServingStatus(cfg.serviceName, healthgrpc.HealthCheckResponse_SERVING)

	go startHTTPServer(cfg, httpHandler, logger, errs)
	go startGRPCServer(cfg, hs, grpcServer, logger, errs)

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
	return cfg
}

func NewServer(cfg config, logger log.Logger) (pb.AddsvcServer, http.Handler) {
	var tracer stdopentracing.Tracer
	{
		tracer = stdopentracing.GlobalTracer()
	}

	var zipkinTracer *zipkin.Tracer
	{
		var (
			err           error
			hostPort      = fmt.Sprintf("localhost:%s", cfg.httpPort)
			serviceName   = cfg.serviceName
			useNoopTracer = (cfg.zipkinV2URL == "")
			reporter      = zipkinhttp.NewReporter(cfg.zipkinV2URL)
		)
		//defer reporter.Close()
		zEP, _ := zipkin.NewEndpoint(serviceName, hostPort)
		zipkinTracer, err = zipkin.NewTracer(reporter, zipkin.WithLocalEndpoint(zEP), zipkin.WithNoopTracer(useNoopTracer))
		if err != nil {
			logger.Log("err", err)
			os.Exit(1)
		}
		if !useNoopTracer {
			logger.Log("tracer", "Zipkin", "type", "Native", "URL", cfg.zipkinV2URL)
		}
	}

	service := service.New(logger)
	endpoints := endpoints.New(service, logger, tracer, zipkinTracer)
	httpHandler := transports.NewHTTPHandler(endpoints, tracer, zipkinTracer, logger)
	grpcServer := transports.MakeGRPCServer(endpoints, tracer, zipkinTracer, logger)

	return grpcServer, httpHandler
}

func startHTTPServer(cfg config, httpHandler http.Handler, logger log.Logger, errs chan error) {
	p := fmt.Sprintf(":%s", cfg.httpPort)
	level.Info(logger).Log("serviceName", cfg.serviceName, "protocol", "HTTP", "exposed", cfg.httpPort)
	errs <- http.ListenAndServe(p, httpHandler)
}

func startGRPCServer(cfg config, hs *health.Server, grpcServer pb.AddsvcServer, logger log.Logger, errs chan error) {
	p := fmt.Sprintf(":%s", cfg.grpcPort)
	listener, err := net.Listen("tcp", p)
	if err != nil {
		level.Error(logger).Log("serviceName", cfg.serviceName, "protocol", "GRPC", "listen", cfg.grpcPort, "err", err)
		os.Exit(1)
	}

	var server *grpc.Server
	level.Info(logger).Log("serviceName", cfg.serviceName, "protocol", "GRPC", "exposed", cfg.grpcPort)
	server = grpc.NewServer(grpc.UnaryInterceptor(kitgrpc.Interceptor))
	pb.RegisterAddsvcServer(server, grpcServer)
	healthgrpc.RegisterHealthServer(server, hs)
	reflection.Register(server)
	errs <- server.Serve(listener)
}
