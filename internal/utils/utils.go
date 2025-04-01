package utils

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/kkuyumjyan/k8s-event-driven-secrets/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var utilsLog = ctrl.Log.WithName("utils")

func AnnotateEventDrivenSecrets(ctx context.Context, eventDrivenSecrets []v1alpha1.EventDrivenSecret, kubeClient client.Client) {
	log := utilsLog.WithName("AnnotateEventDrivenSecrets")
	for _, eds := range eventDrivenSecrets {
		log.Info("✏️ Updating EventDrivenSecret to trigger reconcile", "EventDrivenSecret", eds.Name)

		eds.Annotations["edsm.io/last-updated"] = time.Now().Format(time.RFC3339) // ✅ Trigger reconcile

		if err := kubeClient.Update(ctx, &eds); err != nil {
			log.Error(err, "❌ Failed to update EventDrivenSecret", "EventDrivenSecret", eds.Name)
		} else {
			log.Info("✅ EventDrivenSecret updated successfully", "EventDrivenSecret", eds.Name)
		}
	}
}

// ✅ Hash function using SHA256
// Normalize the secret data and compute SHA256 hash
func CalculateSHA256(secretData map[string][]byte) string {
	// Convert map to sorted slice for deterministic JSON encoding
	keys := make([]string, 0, len(secretData))
	for key := range secretData {
		keys = append(keys, key)
	}
	sort.Strings(keys) // Ensure consistent ordering

	normalized := make(map[string]string)
	for _, key := range keys {
		normalized[key] = string(secretData[key]) // Convert bytes to string
	}

	// ✅ Encode JSON in a consistent format (ensures deterministic hashing)
	jsonData, _ := json.Marshal(normalized)

	// ✅ Compute SHA256 hash
	hash := sha256.Sum256(jsonData)
	return hex.EncodeToString(hash[:])
}

func CompareSecretData(currentData map[string][]byte, expectedData map[string][]byte) bool {
	if len(currentData) != len(expectedData) {
		return false
	}

	for key, expectedValue := range expectedData {
		currentValue, exists := currentData[key]
		if !exists || !bytes.Equal(currentValue, expectedValue) {
			return false
		}
	}
	return true
}

// ✅ Compute SHA256 checksums of all secret values
func ComputeSecretChecksums(ctx context.Context, kubeClient client.Client, namespace string, secretNames []string) (map[string]string, error) {
	log := utilsLog.WithName("ComputeSecretChecksums")

	checksums := make(map[string]string)

	for _, secretName := range secretNames {
		secret := &corev1.Secret{}
		err := kubeClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretName}, secret)
		if err != nil {
			log.Error(err, "failed to get secret", "secretName", secretName)
			return nil, err
		}

		// 🛑 Debugging: Log fetched secret data
		log.Info("🔍 Fetched Kubernetes Secret", "secretName", secretName)

		// ✅ Normalize the secret data into a consistent format before hashing
		checksum := CalculateSHA256(secret.Data)
		checksums[secretName] = checksum

		// 🛑 Debugging: Log calculated hash
		log.Info("🔍 Calculated Secret Hash", "secretName", secretName, "hash", checksum)
	}

	return checksums, nil
}

// ✅ Ensure a Kubernetes Secret exists and is up to date
func CreateOrUpdateK8sSecret(ctx context.Context, k8sClient client.Client, namespace, secretName string, secretData map[string][]byte, updatedSecrets *sync.Map) error {
	log := utilsLog.WithName("CreateOrUpdateK8sSecret")
	secretKey := client.ObjectKey{Name: secretName, Namespace: namespace}
	var existingSecret corev1.Secret

	for i := 0; i < 5; i++ {
		err := k8sClient.Get(ctx, secretKey, &existingSecret)
		if err == nil {
			// ✅ Secret exists, update it
			existingSecret.Data = secretData
			if existingSecret.Labels == nil {
				existingSecret.Labels = map[string]string{}
			}
			existingSecret.Labels["eventdrivensecretsmanaged"] = "true"

			err = k8sClient.Update(ctx, &existingSecret)
			if err == nil {
				log.Info("🔄 Updated existing Kubernetes Secret", "secretname", secretName)
				return nil
			}
			if apierrors.IsConflict(err) {
				log.Info("⚠️ Conflict updating secret, retrying...", "secretname", secretName)
				time.Sleep(200 * time.Millisecond)
				continue
			}

			log.Error(err, "❌ Failed to update existing Kubernetes Secret")
			return err
		}

		if !apierrors.IsNotFound(err) {
			log.Error(err, "❌ Failed to get Kubernetes Secret")
			return err
		}

		// ❌ NotFound — try to create it
		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Labels: map[string]string{
					"eventdrivensecretsmanaged": "true",
				},
			},
			Data: secretData,
			Type: corev1.SecretTypeOpaque,
		}

		err = k8sClient.Create(ctx, newSecret)
		if err == nil {
			log.Info("✅ Created new Kubernetes Secret", "secretname", secretName)
			return nil
		}

		if apierrors.IsAlreadyExists(err) {
			log.Info("⚠️ Secret already exists, retrying get-update flow", "secretname", secretName)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		log.Error(err, "❌ Failed to create new Kubernetes Secret")
		return err
	}

	return fmt.Errorf("❌ Failed to update or create Kubernetes Secret '%s' after multiple retries", secretName)
}

// ✅ Compare stored vs. new checksums with detailed logging
func IsSecretChanged(storedChecksums, newChecksums map[string]string) bool {
	log := utilsLog.WithName("IsSecretChanged")

	// If storedChecksums is empty, consider it a change (first-time case)
	if len(storedChecksums) == 0 {
		log.Info("🔄 Stored checksums are empty, considering secret as changed")
		return true
	}

	for key, newHash := range newChecksums {
		storedHash, exists := storedChecksums[key]

		if !exists {
			log.Info("🔍 Secret missing in stored checksums, marking as changed",
				"key", key, "newHash", newHash)
			return true
		}

		if storedHash != newHash {
			log.Info("🔄 Secret checksum changed",
				"key", key, "storedHash", storedHash, "newHash", newHash)
			return true
		}
	}

	// ✅ No changes detected
	log.Info("✅ No changes in secret checksums, skipping rollout")
	return false
}

func MaskSecretData(secretData map[string][]byte) map[string]string {
	maskedData := make(map[string]string)
	for key := range secretData {
		maskedData[key] = "********"
	}
	return maskedData
}

// ✅ Rollout deployments if secrets have changed
func RolloutDeploymentsSecretUpdate(ctx context.Context, kubeClient client.Client, updatedSecretName, namespace string) error {
	log := utilsLog.WithName("RolloutDeployments")

	log.Info("🚀 Calling RolloutDeploymentsSecretUpdate for deployments using", "secret", updatedSecretName, "namespace", namespace)
	// Fetch deployments that reference the updated secret using the index
	var deployments appsv1.DeploymentList
	err := kubeClient.List(ctx, &deployments, client.MatchingFields{
		"metadata.annotations.eventsecrets": updatedSecretName,
	})
	if err != nil {
		log.Error(err, "failed to list deployments")
		return err
	}

	for _, deployment := range deployments.Items {
		// Extract eventsecrets annotation
		secretsAnnotation, exists := deployment.Annotations["eventsecrets"]
		if !exists {
			continue // Skip if annotation is missing
		}

		// Decode JSON list of secrets
		var secretNames []string
		err := json.Unmarshal([]byte(secretsAnnotation), &secretNames)
		if err != nil {
			log.Error(err, "❌ Failed to parse eventsecrets annotation")
			continue
		}

		// ✅ Compute new secret checksums for ALL secrets in the annotation
		newChecksums, err := ComputeSecretChecksums(ctx, kubeClient, deployment.Namespace, secretNames)
		if err != nil {
			log.Error(err, "❌ Failed to compute secret checksums")
			continue
		}

		// ✅ Fetch stored checksums from annotation (handle invalid JSON safely)
		storedChecksums := make(map[string]string)
		if checksumAnnotation, exists := deployment.Annotations["eventsecrets-checksum"]; exists {
			err := json.Unmarshal([]byte(checksumAnnotation), &storedChecksums)
			if err != nil {
				log.Info("⚠️ Skipping deployment due to invalid eventsecrets-checksum JSON",
					"deployment", deployment.Name,
					"error", err.Error(),
				)
				continue
			}
		}

		// ✅ Compare all checksums
		if !IsSecretChanged(storedChecksums, newChecksums) {
			log.Info("✅ No changes in secrets for deployment, skipping rollout",
				"deployment", deployment.Name,
				"storedChecksums", storedChecksums,
				"newChecksums", newChecksums,
			)
			continue
		}

		// ✅ Ensure annotations map exists before updating
		if deployment.Annotations == nil {
			deployment.Annotations = map[string]string{}
		}

		// ✅ Update deployment annotation with new checksum
		newChecksumJSON, _ := json.Marshal(newChecksums)
		deployment.Annotations["eventsecrets-checksum"] = string(newChecksumJSON)

		// ✅ Update annotation to trigger rollout
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = map[string]string{}
		}
		deployment.Spec.Template.Annotations["secret-update-timestamp"] = time.Now().Format(time.RFC3339)

		// ✅ Apply deployment update
		err = kubeClient.Update(ctx, &deployment)
		if err != nil {
			log.Error(err, "❌ Failed to update deployment", "deployment", deployment.Name)
			continue
		}

		log.Info("✅ Rollout triggered for deployment", "deployment", deployment.Name)
	}

	return nil
}

// SetReadyCondition sets the Ready condition on the EventDrivenSecret status
func SetReadyCondition(eds *v1alpha1.EventDrivenSecret, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&eds.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
	})
}
