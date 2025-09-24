# E2E Tests for Kubernetes

[![Release](https://github.com/clementnuss/e2e-tests/actions/workflows/release.yml/badge.svg)](https://github.com/clementnuss/e2e-tests/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

End-to-end tests for validating Kubernetes cluster functionality using the [kubernetes-sigs/e2e-framework](https://github.com/kubernetes-sigs/e2e-framework).

## Overview

This project provides comprehensive e2e tests to validate core Kubernetes functionality including:

- **Deployment & Pod Management** - Basic workload deployment and lifecycle
- **Storage (CSI)** - Persistent volume provisioning and mounting
- **Networking** - Service discovery and pod-to-pod connectivity
- **RBAC** - Role-based access control and security boundaries

All tests include OpenTelemetry metrics collection and are designed to run continuously in production clusters.

## Test Coverage

### 🚀 Deployment Test (`TestDeployment`)
- Creates nginx deployment with Pod Security Standards compliance
- Verifies pod creation and readiness
- Tests basic Kubernetes scheduling and container runtime

### 🗄️ Storage Test (`TestCSIStorage`)
- Provisions PersistentVolumeClaim via CSI driver
- Mounts volume in test pod
- Validates write/read operations
- Confirms volume cleanup

### 🌐 Network Test (`TestNetworkConnectivity`)
- Deploys nginx service with ClusterIP
- Tests pod-to-service connectivity via curl
- Validates DNS resolution and kube-proxy functionality

### 🔐 RBAC Test (`TestRBACPermissions`)
- Creates basic ServiceAccount with minimal permissions
- Validates security boundaries (denied privileged operations)
- Confirms basic API access works (API server version)

## Quick Start

### Prerequisites
- Go 1.25+
- kubectl configured for target cluster
- [Task](https://taskfile.dev/) (optional, for convenience)

### Local Testing
```bash
# Run all tests
go test -v ./tests/

# Or using Task
task test
```

### Container Usage
```bash
# Pull latest image
docker pull ghcr.io/clementnuss/e2e-tests:latest

# Run tests (requires kubeconfig mount)
docker run --rm \
  -v ~/.kube:/root/.kube:ro \
  ghcr.io/clementnuss/e2e-tests:latest
```

### Kubernetes Deployment
```bash
# Deploy CronJob (runs every 15 minutes)
kubectl apply -k k8s/

# Check status
kubectl get cronjob -n e2e-tests
kubectl get jobs -n e2e-tests

# View logs
kubectl logs -n e2e-tests -l job-name=$(kubectl get jobs -n e2e-tests -o name | head -1 | cut -d/ -f2)
```

## Development

### Building

```bash
# Build test binary
task build-linux

# Build container image
task docker-build

# Test locally
task docker-run
```

### Available Commands (Task)

```bash
task                    # Show all available tasks
task test              # Run tests locally
task build             # Build test binary
task build-linux       # Build Linux binaries (amd64/arm64)
task docker-build      # Build Docker image
task docker-run        # Run container locally
task k8s-deploy        # Deploy to Kubernetes
task k8s-status        # Check deployment status
task k8s-logs          # View latest logs
task clean             # Clean build artifacts
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `OTEL_SERVICE_NAME` | OpenTelemetry service name | `e2e-tests` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP metrics endpoint | _(disabled)_ |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | OTLP protocol (`grpc` or `http/protobuf`) | `grpc` |
| `OTEL_EXPORTER_OTLP_INSECURE` | Use insecure OTLP connection | `false` |

### Kubernetes Configuration

The included Kubernetes manifests (`k8s/`) provide:

- **Namespace**: `e2e-tests`
- **ServiceAccount**: With minimal required permissions
- **CronJob**: Runs tests every 15 minutes
- **RBAC**: ClusterRole for test operations

Update `k8s/cronjob.yaml` to configure:
- Schedule (default: every 15 minutes)
- OTLP endpoint for your monitoring system
- Resource limits

## Metrics

Tests automatically collect OpenTelemetry metrics:

- `test_duration_seconds` (Histogram) - Test execution time
- `test_executed_total` (Counter) - Number of test runs
- `test_errors_total` (Counter) - Number of test failures

### VictoriaMetrics Integration

Configure OTLP endpoint to send metrics to VictoriaMetrics:

```yaml
env:
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: "http://victoria-metrics-otlp:4318"
  - name: OTEL_EXPORTER_OTLP_PROTOCOL
    value: "http/protobuf"
```

## CI/CD

The project uses GoReleaser with GitHub Actions:

- **Snapshot builds** on push to `main`
- **Tagged releases** on version tags (`v*`)
- **Multi-platform** container images (linux/amd64, linux/arm64)
- **Fast builds** with Go module caching and pre-compiled binaries

### Release Process

```bash
# Create and push tag
git tag v1.0.0
git push origin v1.0.0

# GitHub Actions will:
# 1. Build test binaries
# 2. Create container images
# 3. Push to ghcr.io/clementnuss/e2e-tests
```

## Security

All test pods follow Kubernetes Pod Security Standards (restricted):

- Run as non-root user
- No privilege escalation
- Drop all capabilities
- Use seccomp runtime default profile
- Set proper security contexts

## Troubleshooting

### Common Issues

**Tests fail with RBAC errors**: Check that the e2e-tests ServiceAccount has required permissions.

**Storage tests fail**: Verify CSI driver is installed and default StorageClass exists.

**Network tests fail**: Check CNI plugin and kube-proxy configuration.

**Metrics not appearing**: Verify OTLP endpoint configuration and network connectivity.

### Debugging

```bash
# Check test logs
kubectl logs -n e2e-tests -l app=e2e-tests

# Run single test locally
go test -v ./tests/ -run TestDeployment

# Debug with verbose output
go test -v ./tests/ -test.v
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add or modify tests
4. Ensure all tests pass
5. Submit a pull request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built with [kubernetes-sigs/e2e-framework](https://github.com/kubernetes-sigs/e2e-framework)
- Uses [OpenTelemetry](https://opentelemetry.io/) for observability
- Inspired by Kubernetes conformance tests
