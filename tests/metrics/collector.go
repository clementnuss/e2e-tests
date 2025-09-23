package metrics

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var meter = otel.Meter("e2e-tests")

// Collector handles all metrics collection for e2e tests
type Collector struct {
	testDuration metric.Float64Histogram
	testExecuted metric.Int64Counter
	testErrors   metric.Int64Counter
	initialized  bool
}

// NewCollector creates a new metrics collector
func NewCollector() (*Collector, error) {
	c := &Collector{}

	var err error

	// Create test duration histogram
	c.testDuration, err = meter.Float64Histogram(
		"test_duration_seconds",
		metric.WithDescription("Duration of test execution in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create test_duration_seconds histogram: %w", err)
	}

	// Create test executed counter
	c.testExecuted, err = meter.Int64Counter(
		"test_executed_total",
		metric.WithDescription("Total number of tests executed"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create test_executed_total counter: %w", err)
	}

	// Create test errors counter
	c.testErrors, err = meter.Int64Counter(
		"test_errors_total",
		metric.WithDescription("Total number of test errors"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create test_errors_total counter: %w", err)
	}

	c.initialized = true
	log.Println("Metrics collector initialized successfully")
	return c, nil
}

// RecordTestExecution records metrics for a test execution
func (c *Collector) RecordTestExecution(ctx context.Context, t *testing.T, duration time.Duration) {
	testName := t.Name()

	if !c.initialized {
		log.Printf("Warning: metrics collector not initialized, skipping metrics for test %s", testName)
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("test_name", testName),
	}

	c.testExecuted.Add(ctx, 1, metric.WithAttributes(attrs...))
	c.testDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))

	if t.Failed() {
		c.testErrors.Add(ctx, 1, metric.WithAttributes(attrs...))
		log.Printf("Recorded test error for %s", testName)
	}

	log.Printf("Recorded metrics for test %s: duration=%.3fs", testName, duration.Seconds())
}

