package main

import (
	"context"
	"log"
	"os"
	"testing"

	_ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	"sigs.k8s.io/e2e-framework/klient/conf"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"

	"github.com/clementnuss/e2e-tests/tests/metrics"
)

var (
	testenv          env.Environment
	metricsCollector *metrics.Collector
	metricsShutdown  func(context.Context) error
	testContext      context.Context
)

func TestMain(m *testing.M) {
	var exitCode int

	// Initialize metrics
	config := metrics.NewConfigFromEnv()
	shutdown, err := metrics.SetupMetrics(config)
	if err != nil {
		log.Printf("Failed to setup metrics: %v", err)
		os.Exit(1)
	}
	metricsShutdown = shutdown

	// Initialize metrics collector
	metricsCollector, err = metrics.NewCollector()
	if err != nil {
		log.Printf("Failed to create metrics collector: %v", err)
		os.Exit(1)
	}

	// Setup test environment
	testenv = env.New()
	path := conf.ResolveKubeConfigFile()
	cfg := envconf.NewWithKubeConfig(path)
	testenv = env.NewWithConfig(cfg)
	namespace := envconf.RandomName("sample-ns", 16)
	testenv.Setup(
		envfuncs.CreateNamespace(namespace),
	)
	testenv.Finish(
		envfuncs.DeleteNamespace(namespace),
	)

	// Run tests
	exitCode = testenv.Run(m)

	// Shutdown metrics pipeline
	if metricsShutdown != nil {
		ctx := context.Background()
		if err := metricsShutdown(ctx); err != nil {
			log.Printf("Failed to shutdown metrics: %v", err)
		}
	}

	os.Exit(exitCode)
}
