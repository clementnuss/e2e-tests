package main

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestRBACPermissions(t *testing.T) {
	start := time.Now()
	serviceAccountKey := any("serviceaccount-key")

	t.Cleanup(func() {
		metricsCollector.RecordTestExecution(testContext, t, time.Since(start))
	})

	rbacFeature := features.New("rbac/permissions").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create a basic ServiceAccount (no special permissions)
			sa := newRBACServiceAccount(cfg.Namespace(), "rbac-test-sa")
			if err := cfg.Client().Resources().Create(ctx, sa); err != nil {
				t.Fatal(err)
			}
			ctx = context.WithValue(ctx, serviceAccountKey, sa)

			return ctx
		}).
		Assess("rbac restrictions", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			sa := ctx.Value(serviceAccountKey).(*corev1.ServiceAccount)

			// Test 1: Try to list all namespaces (should fail)
			t.Log("Testing: ServiceAccount should NOT be able to list all namespaces")
			namespacePod := newRBACTestPod(cfg.Namespace(), "rbac-test-namespaces", sa.Name,
				"kubectl get namespaces")

			if err := runRBACTestPod(ctx, cfg.Client().Resources(), namespacePod); err != nil {
				t.Fatal(err)
			}

			if !podFailedAsExpected(ctx, cfg.Client().Resources(), namespacePod) {
				t.Fatal("ServiceAccount should not be able to list all namespaces, but it succeeded")
			}
			t.Log("✓ ServiceAccount correctly denied access to list namespaces")

			// Test 2: Try to create a secret in kube-system (should fail)
			t.Log("Testing: ServiceAccount should NOT be able to create secrets in kube-system")
			secretPod := newRBACTestPod(cfg.Namespace(), "rbac-test-secret", sa.Name,
				"kubectl create secret generic test-secret --from-literal=key=value -n kube-system")

			if err := runRBACTestPod(ctx, cfg.Client().Resources(), secretPod); err != nil {
				t.Fatal(err)
			}

			if !podFailedAsExpected(ctx, cfg.Client().Resources(), secretPod) {
				t.Fatal("ServiceAccount should not be able to create secrets in kube-system, but it succeeded")
			}
			t.Log("✓ ServiceAccount correctly denied access to create secrets in kube-system")

			// Test 3: Try to delete nodes (should fail)
			t.Log("Testing: ServiceAccount should NOT be able to list/delete nodes")
			nodesPod := newRBACTestPod(cfg.Namespace(), "rbac-test-nodes", sa.Name,
				"kubectl get nodes")

			if err := runRBACTestPod(ctx, cfg.Client().Resources(), nodesPod); err != nil {
				t.Fatal(err)
			}

			if !podFailedAsExpected(ctx, cfg.Client().Resources(), nodesPod) {
				t.Fatal("ServiceAccount should not be able to list nodes, but it succeeded")
			}
			t.Log("✓ ServiceAccount correctly denied access to list nodes")

			// Test 4: Get API server version (should succeed - basic discovery)
			t.Log("Testing: ServiceAccount should be able to get API server version")
			versionPod := newRBACTestPod(cfg.Namespace(), "rbac-test-version", sa.Name,
				"kubectl get --raw /version")

			if err := runRBACTestPod(ctx, cfg.Client().Resources(), versionPod); err != nil {
				t.Fatal(err)
			}

			if podFailedAsExpected(ctx, cfg.Client().Resources(), versionPod) {
				t.Fatal("ServiceAccount should be able to get API server version, but it failed")
			}
			t.Log("✓ ServiceAccount can get API server version")

			// Test 5: Try basic operations within its own namespace (should succeed or fail depending on cluster policy)
			t.Log("Testing: ServiceAccount should be able to get basic info about itself")
			selfPod := newRBACTestPod(cfg.Namespace(), "rbac-test-self", sa.Name,
				"kubectl get serviceaccount/"+sa.Name)

			if err := runRBACTestPod(ctx, cfg.Client().Resources(), selfPod); err != nil {
				t.Fatal(err)
			}

			if podFailedAsExpected(ctx, cfg.Client().Resources(), selfPod) {
				t.Log("⚠ ServiceAccount cannot get its own info (this may be expected in restrictive clusters)")
			} else {
				t.Log("✓ ServiceAccount can get basic info about itself")
			}

			// Clean up test pods
			cleanupPods := []*corev1.Pod{namespacePod, secretPod, nodesPod, versionPod, selfPod}
			for _, pod := range cleanupPods {
				if err := cfg.Client().Resources().Delete(ctx, pod); err != nil {
					t.Logf("Failed to delete test pod %s: %v", pod.Name, err)
				}
			}

			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Delete ServiceAccount
			if sa := ctx.Value(serviceAccountKey).(*corev1.ServiceAccount); sa != nil {
				if err := cfg.Client().Resources().Delete(ctx, sa); err != nil {
					t.Logf("Failed to delete ServiceAccount: %v", err)
				}
			}

			return ctx
		}).Feature()

	testenv.Test(t, rbacFeature)
}

// newRBACServiceAccount creates a basic ServiceAccount with no special permissions
func newRBACServiceAccount(namespace, name string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": "rbac-test"},
		},
	}
}

// newRBACTestPod creates a pod that runs kubectl commands to test RBAC
func newRBACTestPod(namespace, name, serviceAccountName, command string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": "rbac-test"},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: serviceAccountName,
			RestartPolicy:      corev1.RestartPolicyNever,
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
					Name:  "kubectl-test",
					Image: "bitnami/kubectl:latest",
					Command: []string{
						"sh", "-c",
						command,
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
	}
}

// runRBACTestPod runs a test pod and waits for completion
func runRBACTestPod(ctx context.Context, client *resources.Resources, pod *corev1.Pod) error {
	if err := client.Create(ctx, pod); err != nil {
		return err
	}

	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		var currentPod corev1.Pod
		if err := client.Get(ctx, pod.Name, pod.Namespace, &currentPod); err != nil {
			return false, err
		}

		switch currentPod.Status.Phase {
		case corev1.PodSucceeded, corev1.PodFailed:
			return true, nil
		default:
			return false, nil
		}
	})
}

// podFailedAsExpected checks if a pod failed (which is expected for RBAC denial tests)
func podFailedAsExpected(ctx context.Context, client *resources.Resources, pod *corev1.Pod) bool {
	var currentPod corev1.Pod
	if err := client.Get(ctx, pod.Name, pod.Namespace, &currentPod); err != nil {
		return false
	}

	// Check if pod failed
	if currentPod.Status.Phase == corev1.PodFailed {
		return true
	}

	// Check if container exited with non-zero code (permission denied)
	if len(currentPod.Status.ContainerStatuses) > 0 {
		containerStatus := currentPod.Status.ContainerStatuses[0]
		if containerStatus.State.Terminated != nil {
			return containerStatus.State.Terminated.ExitCode != 0
		}
	}

	return false
}

