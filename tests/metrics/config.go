package metrics

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const (
	defaultServiceName    = "e2e-tests"
	defaultServiceVersion = "0.1.0"
	shutdownTimeout       = 1 * time.Second
)

// Config holds the OpenTelemetry configuration
type Config struct {
	ServiceName    string
	ServiceVersion string
	Endpoint       string
	Headers        map[string]string
	UseHTTP        bool
	Insecure       bool
}

// NewConfigFromEnv creates a new config from environment variables
func NewConfigFromEnv() *Config {
	config := &Config{
		ServiceName:    getEnv("OTEL_SERVICE_NAME", defaultServiceName),
		ServiceVersion: getEnv("OTEL_SERVICE_VERSION", defaultServiceVersion),
		Endpoint:       getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		UseHTTP:        getEnv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc") == "http/protobuf",
		Insecure:       getEnv("OTEL_EXPORTER_OTLP_INSECURE", "false") == "true",
		Headers:        make(map[string]string),
	}

	// Parse headers from OTEL_EXPORTER_OTLP_HEADERS
	if headersStr := os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"); headersStr != "" {
		// Simple parsing of "key1=value1,key2=value2" format
		// For production use, consider a more robust parser
		log.Printf("Parsing OTLP headers: %s", headersStr)
	}

	return config
}

// SetupMetrics initializes the OpenTelemetry metrics pipeline
func SetupMetrics(config *Config) (func(context.Context) error, error) {
	// Create resource with service information
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Skip OTLP setup if no endpoint is configured
	if config.Endpoint == "" {
		log.Println("No OTLP endpoint configured, metrics will be collected but not exported")

		// Create a basic meter provider without exporter for local testing
		mp := metric.NewMeterProvider(
			metric.WithResource(res),
		)
		otel.SetMeterProvider(mp)

		return func(ctx context.Context) error {
			return mp.Shutdown(ctx)
		}, nil
	}

	// Create OTLP exporter
	var exporter metric.Exporter
	if config.UseHTTP {
		opts := []otlpmetrichttp.Option{
			otlpmetrichttp.WithEndpointURL(config.Endpoint),
		}
		if config.Insecure {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}
		exporter, err = otlpmetrichttp.New(context.Background(), opts...)
	} else {
		opts := []otlpmetricgrpc.Option{
			otlpmetricgrpc.WithEndpoint(config.Endpoint),
		}
		if config.Insecure {
			opts = append(opts, otlpmetricgrpc.WithInsecure())
		}
		exporter, err = otlpmetricgrpc.New(context.Background(), opts...)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create meter provider with periodic reader
	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(
			exporter,
			metric.WithInterval(5*time.Second),
		)),
	)

	// Set the global meter provider
	otel.SetMeterProvider(mp)

	log.Printf("Metrics pipeline initialized: endpoint=%s, protocol=%s",
		config.Endpoint,
		map[bool]string{true: "http/protobuf", false: "grpc"}[config.UseHTTP])

	// Return shutdown function
	return func(ctx context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()

		log.Println("Shutting down metrics pipeline...")
		if err := mp.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("failed to shutdown meter provider: %w", err)
		}
		log.Println("Metrics pipeline shutdown complete")
		return nil
	}, nil
}

// getEnv returns the value of an environment variable or a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

