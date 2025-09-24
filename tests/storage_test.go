package main

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestCSIStorage(t *testing.T) {
	start := time.Now()
	pvcKey := any("pvc-key")
	podKey := any("pod-key")

	t.Cleanup(func() {
		metricsCollector.RecordTestExecution(testContext, t, time.Since(start))
	})

	storageFeature := features.New("csi/storage").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create PVC
			pvc := newPVC(cfg.Namespace(), "test-storage-pvc")
			if err := cfg.Client().Resources().Create(ctx, pvc); err != nil {
				t.Fatal(err)
			}
			ctx = context.WithValue(ctx, pvcKey, pvc)

			// Wait for PVC to be bound
			if err := waitForPVCBound(ctx, cfg.Client().Resources(), pvc); err != nil {
				t.Fatalf("PVC not bound: %v", err)
			}

			// Create Pod
			pod := newStoragePod(cfg.Namespace(), "test-storage-pod", "test-storage-pvc")
			if err := cfg.Client().Resources().Create(ctx, pod); err != nil {
				t.Fatal(err)
			}
			ctx = context.WithValue(ctx, podKey, pod)

			// Wait for Pod to complete
			if err := waitForPodCompletion(ctx, cfg.Client().Resources(), pod); err != nil {
				t.Fatalf("Pod did not complete: %v", err)
			}

			return ctx
		}).
		Assess("storage functionality", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			pod := ctx.Value(podKey).(*corev1.Pod)

			// Verify pod completed successfully (exit code 0)
			var currentPod corev1.Pod
			if err := cfg.Client().Resources().Get(ctx, pod.Name, cfg.Namespace(), &currentPod); err != nil {
				t.Fatal(err)
			}

			if currentPod.Status.Phase != corev1.PodSucceeded {
				t.Fatalf("Pod did not succeed: phase is %s", currentPod.Status.Phase)
			}

			// Check container exit code
			if len(currentPod.Status.ContainerStatuses) > 0 {
				containerStatus := currentPod.Status.ContainerStatuses[0]
				if containerStatus.State.Terminated == nil {
					t.Fatal("Container not terminated")
				}
				if containerStatus.State.Terminated.ExitCode != 0 {
					t.Fatalf("Container exited with non-zero code: %d", containerStatus.State.Terminated.ExitCode)
				}
			}

			t.Logf("Pod %s completed successfully (exit code 0)", currentPod.Name)

			// Verify PVC is bound
			pvc := ctx.Value(pvcKey).(*corev1.PersistentVolumeClaim)
			var currentPvc corev1.PersistentVolumeClaim
			if err := cfg.Client().Resources().Get(ctx, pvc.Name, cfg.Namespace(), &currentPvc); err != nil {
				t.Fatal(err)
			}

			if currentPvc.Status.Phase != corev1.ClaimBound {
				t.Fatalf("PVC not bound: expected phase %s, got %s", corev1.ClaimBound, currentPvc.Status.Phase)
			}

			t.Logf("PVC %s is bound to volume %s", currentPvc.Name, currentPvc.Spec.VolumeName)

			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Delete Pod
			if pod := ctx.Value(podKey).(*corev1.Pod); pod != nil {
				if err := cfg.Client().Resources().Delete(ctx, pod); err != nil {
					t.Logf("Failed to delete Pod: %v", err)
				}
			}

			// Delete PVC
			if pvc := ctx.Value(pvcKey).(*corev1.PersistentVolumeClaim); pvc != nil {
				if err := cfg.Client().Resources().Delete(ctx, pvc); err != nil {
					t.Logf("Failed to delete PVC: %v", err)
				}
			}

			return ctx
		}).Feature()

	testenv.Test(t, storageFeature)
}

// newPVC creates a new PersistentVolumeClaim
func newPVC(namespace, name string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": "test-storage"},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}
}

// newStoragePod creates a Pod that writes data to mounted storage
func newStoragePod(namespace, name, pvcName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": "test-storage"},
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
					Name:  "storage-test",
					Image: "alpine:latest",
					Command: []string{
						"sh", "-c",
						"echo 'CSI storage test data' > /data/test-file.txt && " +
							"cat /data/test-file.txt && " +
							"echo 'Storage test completed successfully'",
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
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "data",
							MountPath: "/data",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
		},
	}
}

// waitForPVCBound waits for a PVC to be bound
func waitForPVCBound(ctx context.Context, client *resources.Resources, pvc *corev1.PersistentVolumeClaim) error {
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		var currentPvc corev1.PersistentVolumeClaim
		if err := client.Get(ctx, pvc.Name, pvc.Namespace, &currentPvc); err != nil {
			return false, err
		}

		return currentPvc.Status.Phase == corev1.ClaimBound, nil
	})
}

// waitForPodCompletion waits for a Pod to complete successfully
func waitForPodCompletion(ctx context.Context, client *resources.Resources, pod *corev1.Pod) error {
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		var currentPod corev1.Pod
		if err := client.Get(ctx, pod.Name, pod.Namespace, &currentPod); err != nil {
			return false, err
		}

		switch currentPod.Status.Phase {
		case corev1.PodSucceeded:
			return true, nil
		case corev1.PodFailed:
			return false, nil
		default:
			return false, nil
		}
	})
}

