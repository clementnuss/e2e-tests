package main

import (
	"context"
	"log"
	"os"
	"runtime/debug"
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

	// Log build information
	logBuildInfo()

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

// logBuildInfo logs version and build information using debug.ReadBuildInfo()
func logBuildInfo() {
	log.Printf("=== E2E Tests Starting ===")

	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		log.Printf("Go version: %s", buildInfo.GoVersion)
		log.Printf("Module path: %s", buildInfo.Main.Path)
		if buildInfo.Main.Version != "(devel)" {
			log.Printf("Module version: %s", buildInfo.Main.Version)
		}

		// Extract VCS information from build settings
		var revision, time, modified string
		for _, setting := range buildInfo.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.time":
				time = setting.Value
			case "vcs.modified":
				modified = setting.Value
			}
		}

		if revision != "" {
			log.Printf("Git revision: %s", revision)
		}
		if time != "" {
			log.Printf("Git time: %s", time)
		}
		if modified == "true" {
			log.Printf("Modified: true (uncommitted changes)")
		}
	} else {
		log.Printf("Build info not available")
	}

	log.Printf("========================")
}
