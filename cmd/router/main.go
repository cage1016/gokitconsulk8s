package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	stdopentracing "github.com/opentracing/opentracing-go"
	"github.com/openzipkin/zipkin-go"
	zipkinhttp "github.com/openzipkin/zipkin-go/reporter/http"

	routertransport "github.com/cage1016/gokitsonsulk8s/pkg/router/transport"
)

const (
	defZipkinV2URL   = ""
	defNameSpace     = "gokitconsulk8s"
	defServiceName   = "router"
	defLogLevel      = "error"
	defHTTPPort      = "8000"
	defGRPCPort      = "8001"
	defRretryTimeout = "500" // time.Millisecond
	defRretryMax     = "3"
	defAddsvcURL     = ""
	defFoosvcURL     = ""

	envZipkinV2URL  = "QS_ZIPKIN_V2_URL"
	envNameSpace    = "QS_ROUTER_NAMESPACE"
	envServiceName  = "QS_ROUTER_SERVICE_NAME"
	envLogLevel     = "QS_ROUTER_LOG_LEVEL"
	envHTTPPort     = "QS_ROUTER_HTTP_PORT"
	envGRPCPort     = "QS_ROUTER_GRPC_PORT"
	envRetryMax     = "QS_ROUTER_RETRY_MAX"
	envRetryTimeout = "QS_ROUTER_RETRY_TIMEOUT"
	envAddsvcURL    = "QS_ADDSVC_URL"
	envFoosvcURL    = "QS_FOOSVC_URL"
)

// Env reads specified environment variable. If no value has been found,
// fallback is returned.
func env(key string, fallback string) (s0 string) {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type config struct {
	nameSpace    string
	serviceName  string
	logLevel     string
	serviceHost  string
	httpPort     string
	grpcPort     string
	zipkinV2URL  string
	retryMax     int64
	retryTimeout int64
	addsvcURL    string
	foosvcURL    string
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
		defer reporter.Close()
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

	ctx := context.Background()
	errs := make(chan error, 1)

	r := routertransport.MakeHandler(ctx, cfg.addsvcURL, cfg.foosvcURL, cfg.retryMax, cfg.retryMax, tracer, zipkinTracer, logger)

	go startHTTPServer(nil, r, cfg.httpPort, logger, errs)
	//go startGRPCServer(consultAddress, tracer, zipkinTracer, cfg.grpcPort, cfg.serverCert, cfg.serverKey, logger, errs)

	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT)
		errs <- fmt.Errorf("%s", <-c)
	}()

	errc := <-errs
	level.Info(logger).Log("serviceName", cfg.serviceName, "terminated", errc)
}

func loadConfig(logger log.Logger) (cfg config) {
	retryMax, err := strconv.ParseInt(env(envRetryMax, defRretryMax), 10, 0)
	if err != nil {
		level.Error(logger).Log("envRetryMax", envRetryMax, "error", err)
	}

	retryTimeout, err := strconv.ParseInt(env(envRetryTimeout, defRretryTimeout), 10, 0)
	if err != nil {
		level.Error(logger).Log("envRetryTimeout", envRetryTimeout, "error", err)
	}

	cfg.serviceName = env(envServiceName, defServiceName)
	cfg.logLevel = env(envLogLevel, defLogLevel)
	cfg.httpPort = env(envHTTPPort, defHTTPPort)
	cfg.grpcPort = env(envGRPCPort, defGRPCPort)
	cfg.zipkinV2URL = env(envZipkinV2URL, defZipkinV2URL)
	cfg.retryMax = retryMax
	cfg.retryTimeout = retryTimeout
	cfg.addsvcURL = env(envAddsvcURL, defAddsvcURL)
	cfg.foosvcURL = env(envFoosvcURL, defFoosvcURL)
	return
}

func startHTTPServer(tlsConfig *tls.Config, handler http.Handler, port string, logger log.Logger, errs chan error) {
	p := fmt.Sprintf(":%s", port)
	level.Info(logger).Log("protocol", "HTTP", "exposed", port)
	errs <- http.ListenAndServe(p, handler)
}
