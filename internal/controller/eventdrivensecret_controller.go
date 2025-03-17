/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	providers "github.com/kkuyumjyan/k8s-event-driven-secrets/internal/providers"
	utils "github.com/kkuyumjyan/k8s-event-driven-secrets/internal/utils"

	secretsv1alpha1 "github.com/kkuyumjyan/k8s-event-driven-secrets/api/v1alpha1"
)

var (
	activeListeners = make(map[string]bool)
	listenersLock   = &sync.Mutex{}
)

// EventDrivenSecretReconciler reconciles a EventDrivenSecret object
type EventDrivenSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=secrets.edsm.io,resources=eventdrivensecrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=secrets.edsm.io,resources=eventdrivensecrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=secrets.edsm.io,resources=eventdrivensecrets/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the EventDrivenSecret object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile

func (r *EventDrivenSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	fmt.Println("Reconciling EventDrivenSecret:", req.NamespacedName)

	// Fetch the EventDrivenSecret resource.
	var eventDrivenSecret secretsv1alpha1.EventDrivenSecret
	if err := r.Get(ctx, req.NamespacedName, &eventDrivenSecret); err != nil {
		fmt.Println("Failed to get EventDrivenSecret:", err)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Extract values from the resource spec
	cloudProvider := eventDrivenSecret.Spec.CloudProvider
	region := eventDrivenSecret.Spec.Region
	secretPath := eventDrivenSecret.Spec.SecretPath
	targetSecretName := eventDrivenSecret.Spec.TargetSecretName

	fmt.Printf("Detected EventDrivenSecret: CloudProvider=%s, Region=%s, SecretPath=%s. KubernetesSecretName=%s\n", cloudProvider, region, secretPath, targetSecretName)

	// ✅ Ensure only ONE listener per provider
	listenersLock.Lock()
	if _, exists := activeListeners[cloudProvider]; !exists {
		activeListeners[cloudProvider] = true // Mark as running
		listenersLock.Unlock()

		// Start the listener in a separate Goroutine
		go func(provider string) {
			fmt.Printf("🚀 Starting listener for provider: %s\n", provider)

			err := providers.StartListening(context.Background(), provider, r.Client)
			if err != nil {
				fmt.Printf("❌ Listener for provider %s crashed: %v\n", provider, err)
			}

			// 🔄 If the listener exits (unexpectedly), remove it from the active list to allow restart
			listenersLock.Lock()
			delete(activeListeners, provider)
			listenersLock.Unlock()

		}(cloudProvider)

	} else {
		listenersLock.Unlock()
	}

	// Fetch secret from AWS Secrets Manager
	secretValue, err := providers.FetchSecretData(region, secretPath, cloudProvider)
	if err != nil {
		fmt.Println("Failed to get secret value:", err)
		return ctrl.Result{}, err
	}

	// ✅ Fetch the existing Kubernetes Secret
	var targetSecret corev1.Secret
	err = r.Client.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: targetSecretName}, &targetSecret)
	if err != nil {
		if errors.IsNotFound(err) {
			// Secret does not exist -> Recreate it from AWS/GCP
			fmt.Println("🔍 Target secret is missing, recreating...")
		} else {
			// Other errors
			return ctrl.Result{}, err
		}
	}

	// ✅ Compare current secret with expected secret
	if !utils.CompareSecretData(targetSecret.Data, secretValue) {
		fmt.Println("⚠️ Secret modified manually! Restoring original version...")

		// Restore the correct secret
		err := utils.CreateOrUpdateK8sSecret(ctx, r.Client, req.Namespace, targetSecretName, secretValue)
		if err != nil {
			fmt.Println("❌ Failed to restore Kubernetes secret:", err)
			return ctrl.Result{}, err
		}
	} else {
		fmt.Println("✅ Secret is up-to-date, no changes needed.")
	}

	// Rollout the Deployments that reference the secret
	err = utils.RolloutDeploymentsSecretUpdate(ctx, r.Client, targetSecretName, req.NamespacedName.Namespace)
	if err != nil {
		fmt.Println("Failed to rollout deployments:", err)
		return ctrl.Result{}, err
	}

	fmt.Printf("Secret value: %s\n", utils.MaskSecretData(secretValue))

	// Log what we received.
	fmt.Printf("Detected EventDrivenSecret: CloudProvider=%s, Region=%s, SecretPath=%s\n",
		eventDrivenSecret.Spec.CloudProvider, eventDrivenSecret.Spec.Region, eventDrivenSecret.Spec.SecretPath)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EventDrivenSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	var err error
	err = mgr.GetFieldIndexer().IndexField(
		context.TODO(),
		&appsv1.Deployment{},
		"metadata.annotations.eventsecrets",
		func(obj client.Object) []string {
			deployment := obj.(*appsv1.Deployment)
			secretsAnnotation, exists := deployment.Annotations["eventsecrets"]
			if !exists {
				return nil
			}

			var secretList []string
			err := json.Unmarshal([]byte(secretsAnnotation), &secretList)
			if err != nil {
				return nil
			}
			return secretList
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create index for metadata.annotations.eventsecrets: %w", err)
	}

	err = mgr.GetFieldIndexer().IndexField(
		context.TODO(),
		&secretsv1alpha1.EventDrivenSecret{},
		"spec.secretPath",
		func(obj client.Object) []string {
			secret := obj.(*secretsv1alpha1.EventDrivenSecret)
			if secret.Spec.SecretPath == "" {
				return nil
			}
			return []string{secret.Spec.SecretPath}
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create index for spec.secretPath: %w", err)
	}

	err = mgr.GetFieldIndexer().IndexField(
		context.TODO(),
		&secretsv1alpha1.EventDrivenSecret{},
		"spec.targetSecretName",
		func(obj client.Object) []string {
			eds := obj.(*secretsv1alpha1.EventDrivenSecret)
			if eds.Spec.TargetSecretName == "" {
				return nil
			}
			return []string{eds.Spec.TargetSecretName}
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create index for spec.targetSecretName: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&secretsv1alpha1.EventDrivenSecret{}).
		Owns(&corev1.Secret{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				secret, ok := obj.(*corev1.Secret)
				if !ok {
					return nil
				}

				// 🔎 Find matching EventDrivenSecret
				var matchingEDSList secretsv1alpha1.EventDrivenSecretList
				err := mgr.GetClient().List(ctx, &matchingEDSList, client.MatchingFields{"spec.targetSecretName": secret.Name})
				if err != nil {
					return nil
				}

				// 📌 Enqueue all related EventDrivenSecrets
				var requests []reconcile.Request
				for _, eds := range matchingEDSList.Items {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: eds.Namespace,
							Name:      eds.Name,
						},
					})
				}

				return requests
			}),
		).
		Named("eventdrivensecret").
		Complete(r)
}
