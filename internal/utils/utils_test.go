package utils

import (
	"context"
	"encoding/json"

	"sync"
	"testing"
	"time"

	"k8s.io/utils/ptr"

	"github.com/kkuyumjyan/k8s-event-driven-secrets/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	namespace = "default"
)

func TestAnnotateEventDrivenSecrets(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, v1alpha1.AddToScheme(scheme))

	ctx := context.TODO()
	name := "test-eds"

	eds := &v1alpha1.EventDrivenSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: map[string]string{},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(eds).
		Build()

	// Run the function
	AnnotateEventDrivenSecrets(ctx, []v1alpha1.EventDrivenSecret{*eds}, fakeClient)

	// Verify that the annotation was updated
	updatedEDS := &v1alpha1.EventDrivenSecret{}
	err := fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, updatedEDS)
	assert.NoError(t, err)

	val, ok := updatedEDS.Annotations["edsm.io/last-updated"]
	assert.True(t, ok, "Expected annotation to be present")
	_, err = time.Parse(time.RFC3339, val)
	assert.NoError(t, err, "Expected annotation to be a valid RFC3339 timestamp")
}

func TestCalculateSHA256(t *testing.T) {
	data1 := map[string][]byte{
		"username": []byte("admin"),
		"password": []byte("s3cr3t"),
	}
	data2 := map[string][]byte{
		"password": []byte("s3cr3t"),
		"username": []byte("admin"),
	}

	hash1 := CalculateSHA256(data1)
	hash2 := CalculateSHA256(data2)

	assert.Equal(t, hash1, hash2, "Hash should be deterministic and order-independent")

	data3 := map[string][]byte{
		"username": []byte("admin"),
		"password": []byte("different"),
	}

	hash3 := CalculateSHA256(data3)
	assert.NotEqual(t, hash1, hash3, "Different content should result in different hashes")
}

func TestCompareSecretData(t *testing.T) {
	a := map[string][]byte{"key": []byte("value")}
	b := map[string][]byte{"key": []byte("value")}
	assert.True(t, CompareSecretData(a, b))
}

func TestComputeSecretChecksums(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	ctx := context.TODO()
	secretName := "my-secret"

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("123"),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	checksums, err := ComputeSecretChecksums(ctx, client, namespace, []string{secretName})
	assert.NoError(t, err)
	assert.Len(t, checksums, 1)

	expectedHash := CalculateSHA256(secret.Data)
	assert.Equal(t, expectedHash, checksums[secretName])
}

func TestCreateOrUpdateK8sSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	ctx := context.TODO()
	secretName := "my-test-secret"
	data := map[string][]byte{
		"username": []byte("admin"),
		"password": []byte("s3cr3t"),
	}
	updatedSecrets := &sync.Map{}

	// 💡 Create a fake Kubernetes client
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// 🧪 1. Secret should be created successfully
	err := CreateOrUpdateK8sSecret(ctx, fakeClient, namespace, secretName, data, updatedSecrets)
	assert.NoError(t, err, "expected secret to be created")

	// 🧪 2. Secret should now exist with correct data
	var created corev1.Secret
	err = fakeClient.Get(ctx, client.ObjectKey{Name: secretName, Namespace: namespace}, &created)
	assert.NoError(t, err, "expected secret to exist")
	assert.Equal(t, data, created.Data)
	assert.Equal(t, "true", created.Labels["eventdrivensecretsmanaged"])

	// 🧪 3. Now modify and update the secret
	updatedData := map[string][]byte{
		"username": []byte("new-user"),
		"token":    []byte("new-token"),
	}
	err = CreateOrUpdateK8sSecret(ctx, fakeClient, namespace, secretName, updatedData, updatedSecrets)
	assert.NoError(t, err, "expected secret to be updated")

	var updated corev1.Secret
	err = fakeClient.Get(ctx, client.ObjectKey{Name: secretName, Namespace: namespace}, &updated)
	assert.NoError(t, err, "expected updated secret to exist")
	assert.Equal(t, updatedData, updated.Data)
	assert.Equal(t, "true", updated.Labels["eventdrivensecretsmanaged"])
}

func TestIsSecretChanged(t *testing.T) {
	old := map[string]string{
		"my-secret": "abc123",
	}

	// Case 1: Checksum changed
	new1 := map[string]string{
		"my-secret": "def456",
	}
	assert.True(t, IsSecretChanged(old, new1))

	// Case 2: New key added
	new2 := map[string]string{
		"my-secret": "abc123",
		"extra":     "zzz",
	}
	assert.True(t, IsSecretChanged(old, new2))

	// Case 3: Same values
	new3 := map[string]string{
		"my-secret": "abc123",
	}
	assert.False(t, IsSecretChanged(old, new3))

	// Case 4: Empty old map (first-time deployment)
	assert.True(t, IsSecretChanged(map[string]string{}, new3))
}

func TestMaskSecretData(t *testing.T) {
	data := map[string][]byte{
		"username": []byte("admin"),
		"password": []byte("secret"),
	}
	masked := MaskSecretData(data)

	assert.Equal(t, "********", masked["username"])
	assert.Equal(t, "********", masked["password"])
	assert.Len(t, masked, 2)
}

func TestRolloutDeploymentsSecretUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	ctx := context.TODO()
	secretName := "my-secret"
	secretData := map[string][]byte{
		"username": []byte("admin"),
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: secretData,
	}

	secretList := []string{secretName}
	annotationBytes, _ := json.Marshal(secretList)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deployment",
			Namespace: namespace,
			Annotations: map[string]string{
				"eventsecrets": string(annotationBytes),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "test",
						Image: "busybox",
					}},
				},
			},
		},
	}

	// Setup fake client and inject indexer manually
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, deployment).
		WithIndex(&appsv1.Deployment{}, "metadata.annotations.eventsecrets", func(obj client.Object) []string {
			if val, ok := obj.GetAnnotations()["eventsecrets"]; ok {
				var names []string
				_ = json.Unmarshal([]byte(val), &names)
				return names
			}
			return nil
		}).
		Build()

	// Simulate what your manager does: index deployments by secret name
	// This is skipped here — you could add logic to test with real field selectors using envtest if needed.

	// ⚠️ Because the fake client doesn't support `.MatchingFields`, simulate manually by injecting the filtered list
	// Alternative: use envtest or mock List() method if needed.

	// Run the function (this will not filter, but we focus on path correctness)
	err := RolloutDeploymentsSecretUpdate(ctx, fakeClient, secretName, namespace)
	assert.NoError(t, err)

	// Verify the deployment was updated
	updated := &appsv1.Deployment{}
	err = fakeClient.Get(ctx, client.ObjectKey{Name: "my-deployment", Namespace: namespace}, updated)
	assert.NoError(t, err)

	// It should have a new annotation for `eventsecrets-checksum` and `secret-update-timestamp`
	_, checksumExists := updated.Annotations["eventsecrets-checksum"]
	_, timestampExists := updated.Spec.Template.Annotations["secret-update-timestamp"]
	assert.True(t, checksumExists)
	assert.True(t, timestampExists)
}

func TestSetReadyCondition(t *testing.T) {
	eds := &v1alpha1.EventDrivenSecret{}

	status := metav1.ConditionTrue
	reason := "Synced"
	message := "Secret synced successfully"

	SetReadyCondition(eds, status, reason, message)

	// Check that exactly one condition was added
	assert.Len(t, eds.Status.Conditions, 1)

	cond := eds.Status.Conditions[0]
	assert.Equal(t, "Ready", cond.Type)
	assert.Equal(t, status, cond.Status)
	assert.Equal(t, reason, cond.Reason)
	assert.Equal(t, message, cond.Message)

	// Check if LastTransitionTime is within the last few seconds
	now := time.Now()
	assert.WithinDuration(t, now, cond.LastTransitionTime.Time, time.Second*5)
}
