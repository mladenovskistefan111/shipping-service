package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"shipping-service/internal/shipping"
	pb "shipping-service/proto"

	grpcprom "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grafana/pyroscope-go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

var log *logrus.Logger

func init() {
	log = logrus.New()
	log.Formatter = &logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "severity",
			logrus.FieldKeyMsg:   "message",
		},
		TimestampFormat: time.RFC3339Nano,
	}
	log.Out = os.Stdout
}

func main() {
	// --- Tracing (OpenTelemetry → Grafana Tempo via Alloy) ---
	if os.Getenv("ENABLE_TRACING") == "1" {
		if err := initTracing(); err != nil {
			log.Warnf("tracing init failed, continuing without it: %v", err)
		} else {
			log.Info("tracing enabled")
		}
	} else {
		log.Info("tracing disabled — set ENABLE_TRACING=1 to enable")
	}

	// --- Profiling (pprof HTTP + Pyroscope push) ---
	if os.Getenv("ENABLE_PROFILING") == "1" {
		initProfiling()
	} else {
		log.Info("profiling disabled — set ENABLE_PROFILING=1 to enable")
	}

	port := "50051"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	log.Infof("starting grpc server on :%s", port)
	if err := run(port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func run(port string) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %s: %w", port, err)
	}

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	// gRPC server with OTel tracing + Prometheus metrics interceptors
	srv := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.UnaryInterceptor(grpcprom.UnaryServerInterceptor),
		grpc.StreamInterceptor(grpcprom.StreamServerInterceptor),
	)

	svc := shipping.NewService(log)
	pb.RegisterShippingServiceServer(srv, svc)

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(srv, healthSrv)
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	reflection.Register(srv)

	// Register all gRPC server metrics with Prometheus
	grpcprom.Register(srv)
	grpcprom.EnableHandlingTimeHistogram()

	// --- Metrics + pprof on the same HTTP server ---
	go func() {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		metricsMux.Handle("/debug/pprof/", http.DefaultServeMux)
		metricsPort := "9090"
		if p := os.Getenv("METRICS_PORT"); p != "" {
			metricsPort = p
		}
		log.Infof("metrics + pprof endpoint on :%s", metricsPort)
		if err := http.ListenAndServe(":"+metricsPort, metricsMux); err != nil {
			log.Warnf("metrics server error: %v", err)
		}
	}()

	log.Infof("listening on %s", listener.Addr().String())
	return srv.Serve(listener)
}

// initTracing wires OpenTelemetry to an OTLP gRPC collector (Alloy).
func initTracing() error {
	collectorAddr := os.Getenv("COLLECTOR_SERVICE_ADDR")
	if collectorAddr == "" {
		return fmt.Errorf("COLLECTOR_SERVICE_ADDR not set")
	}

	ctx := context.Background()

	conn, err := grpc.NewClient(
		collectorAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to collector %s: %w", collectorAddr, err)
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return fmt.Errorf("failed to create otlp exporter: %w", err)
	}

	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "shipping-service"
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return nil
}

// initProfiling starts the Pyroscope push-based profiler.
func initProfiling() {
	pyroscopeAddr := os.Getenv("PYROSCOPE_ADDR")
	if pyroscopeAddr == "" {
		pyroscopeAddr = "http://pyroscope:4040"
	}

	_, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: "shipping-service",
		ServerAddress:   pyroscopeAddr,
		Logger:          pyroscope.StandardLogger,
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
		},
	})
	if err != nil {
		log.Warnf("pyroscope init failed, continuing without it: %v", err)
		return
	}
	log.Info("profiling enabled → pushing to " + pyroscopeAddr)
}