package main

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestNetworkConnectivity(t *testing.T) {
	start := time.Now()
	deploymentKey := any("deployment-key")
	serviceKey := any("service-key")

	t.Cleanup(func() {
		metricsCollector.RecordTestExecution(testContext, t, time.Since(start))
	})

	networkFeature := features.New("network/connectivity").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create nginx deployment
			deployment := newNetworkDeployment(cfg.Namespace(), "network-test-nginx")
			if err := cfg.Client().Resources().Create(ctx, deployment); err != nil {
				t.Fatal(err)
			}
			ctx = context.WithValue(ctx, deploymentKey, deployment)

			// Wait for deployment to be ready
			if err := waitForDeploymentReady(ctx, cfg.Client().Resources(), deployment); err != nil {
				t.Fatalf("Deployment not ready: %v", err)
			}

			// Create service
			service := newNetworkService(cfg.Namespace(), "network-test-service")
			if err := cfg.Client().Resources().Create(ctx, service); err != nil {
				t.Fatal(err)
			}
			ctx = context.WithValue(ctx, serviceKey, service)

			return ctx
		}).
		Assess("network connectivity", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			service := ctx.Value(serviceKey).(*corev1.Service)

			// Create a temporary client pod to test connectivity
			clientPod := newClientPod(cfg.Namespace(), "network-test-client", service.Name)
			if err := cfg.Client().Resources().Create(ctx, clientPod); err != nil {
				t.Fatal(err)
			}

			// Wait for client pod to complete
			if err := waitForPodCompletion(ctx, cfg.Client().Resources(), clientPod); err != nil {
				t.Fatalf("Client pod did not complete: %v", err)
			}

			// Verify client pod completed successfully (exit code 0)
			var currentPod corev1.Pod
			if err := cfg.Client().Resources().Get(ctx, clientPod.Name, cfg.Namespace(), &currentPod); err != nil {
				t.Fatal(err)
			}

			if currentPod.Status.Phase != corev1.PodSucceeded {
				t.Fatalf("Client pod did not succeed: phase is %s", currentPod.Status.Phase)
			}

			// Check container exit code
			if len(currentPod.Status.ContainerStatuses) > 0 {
				containerStatus := currentPod.Status.ContainerStatuses[0]
				if containerStatus.State.Terminated == nil {
					t.Fatal("Client container not terminated")
				}
				if containerStatus.State.Terminated.ExitCode != 0 {
					t.Fatalf("Client container exited with non-zero code: %d", containerStatus.State.Terminated.ExitCode)
				}
			}

			t.Logf("Network connectivity test passed: client pod successfully connected to service %s", service.Name)

			// Clean up client pod
			if err := cfg.Client().Resources().Delete(ctx, clientPod); err != nil {
				t.Logf("Failed to delete client pod: %v", err)
			}

			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Delete service
			if service := ctx.Value(serviceKey).(*corev1.Service); service != nil {
				if err := cfg.Client().Resources().Delete(ctx, service); err != nil {
					t.Logf("Failed to delete service: %v", err)
				}
			}

			// Delete deployment
			if deployment := ctx.Value(deploymentKey).(*appsv1.Deployment); deployment != nil {
				if err := cfg.Client().Resources().Delete(ctx, deployment); err != nil {
					t.Logf("Failed to delete deployment: %v", err)
				}
			}

			return ctx
		}).Feature()

	testenv.Test(t, networkFeature)
}

// newNetworkDeployment creates an nginx deployment for network testing
func newNetworkDeployment(namespace, name string) *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": "network-test"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "network-test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "network-test"},
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						RunAsUser:    &[]int64{65534}[0], // nobody user
						FSGroup:      &[]int64{65534}[0],
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "cgr.dev/chainguard/nginx",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &[]bool{false}[0],
								RunAsNonRoot:             &[]bool{true}[0],
								RunAsUser:                &[]int64{65534}[0],
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								SeccompProfile: &corev1.SeccompProfile{
									Type: corev1.SeccompProfileTypeRuntimeDefault,
								},
							},
						},
					},
				},
			},
		},
	}
}

// newNetworkService creates a service for the nginx deployment
func newNetworkService(namespace, name string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": "network-test"},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "network-test"},
			Ports: []corev1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt32(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// newClientPod creates a client pod to test network connectivity
func newClientPod(namespace, name, serviceName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": "network-test-client"},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: &[]bool{true}[0],
				RunAsUser:    &[]int64{65534}[0], // nobody user
				FSGroup:      &[]int64{65534}[0],
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "curl-test",
					Image: "curlimages/curl:latest",
					Command: []string{
						"sh", "-c",
						"echo 'Testing network connectivity to " + serviceName + "...' && " +
							"curl -f --max-time 30 --connect-timeout 10 http://" + serviceName + " && " +
							"echo 'Network connectivity test successful'",
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: &[]bool{false}[0],
						RunAsNonRoot:             &[]bool{true}[0],
						RunAsUser:                &[]int64{65532}[0], // curl user
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				},
			},
		},
	}
}

// waitForDeploymentReady waits for a deployment to be ready
func waitForDeploymentReady(ctx context.Context, client *resources.Resources, deployment *appsv1.Deployment) error {
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
		var currentDeployment appsv1.Deployment
		if err := client.Get(ctx, deployment.Name, deployment.Namespace, &currentDeployment); err != nil {
			return false, err
		}

		// Check if all replicas are ready
		return currentDeployment.Status.ReadyReplicas == *currentDeployment.Spec.Replicas, nil
	})
}
