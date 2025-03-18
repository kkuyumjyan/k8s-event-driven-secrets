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

	"github.com/kkuyumjyan/k8s-event-driven-secrets/internal/providers"
	"github.com/kkuyumjyan/k8s-event-driven-secrets/internal/utils"

	secretsv1alpha1 "github.com/kkuyumjyan/k8s-event-driven-secrets/api/v1alpha1"
)

var (
	listenersLock = &sync.Mutex{}
)

// EventDrivenSecretReconciler reconciles a EventDrivenSecret object
type EventDrivenSecretReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	updatedSecrets  sync.Map
	activeProviders sync.Map
	mgr             ctrl.Manager
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
	log := log.FromContext(ctx)

	log.Info("🔄 Reconcile triggered", "namespace", req.Namespace, "name", req.Name)

	// Fetch the EventDrivenSecret resource
	var eventDrivenSecret secretsv1alpha1.EventDrivenSecret
	if err := r.Get(ctx, req.NamespacedName, &eventDrivenSecret); err != nil {
		if errors.IsNotFound(err) {
			log.Info("🗑 EventDrivenSecret deleted, cleaning up target Secret", "name", req.Name)

			// If the EventDrivenSecret is deleted, delete the target Secret
			targetSecret := &corev1.Secret{}
			err := r.Get(ctx, types.NamespacedName{
				Namespace: req.Namespace,
				Name:      eventDrivenSecret.Spec.TargetSecretName,
			}, targetSecret)

			if err == nil {
				log.Info("🗑 Deleting target Secret", "name", targetSecret.Name)
				if err := r.Delete(ctx, targetSecret); err != nil {
					log.Error(err, "❌ Failed to delete target Secret", "name", eventDrivenSecret.Spec.TargetSecretName)
					return ctrl.Result{}, err
				}
			} else if !errors.IsNotFound(err) {
				log.Error(err, "Failed to get target Secret before deletion", "name", eventDrivenSecret.Spec.TargetSecretName)
			}

			return ctrl.Result{}, nil
		}
		log.Error(err, "❌ Failed to get EventDrivenSecret", "name", req.Name)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Extract values from the resource spec
	cloudProvider := eventDrivenSecret.Spec.CloudProvider
	region := eventDrivenSecret.Spec.Region
	secretPath := eventDrivenSecret.Spec.SecretPath
	targetSecretName := eventDrivenSecret.Spec.TargetSecretName

	log.Info("🔎 Detected EventDrivenSecret", "cloudProvider", cloudProvider, "region", region, "secretPath", secretPath, "targetSecretName", targetSecretName)

	// ✅ If a new provider appears, start its listener dynamically instead of restarting `SetupWithManager`
	if _, exists := r.activeProviders.Load(cloudProvider); !exists {
		log.Info("🆕 New provider detected, starting listener!", "provider", cloudProvider)

		// Mark provider as running
		r.activeProviders.Store(cloudProvider, true)

		// Start provider listener in a separate goroutine
		go func() {
			providers.StartListening(ctx, cloudProvider, r.Client, &r.updatedSecrets)
			log.Info("✅ Listener started for provider", "provider", cloudProvider)
		}()
	}

	// Fetch secret from AWS Secrets Manager
	secretValue, err := providers.FetchSecretData(ctx, region, secretPath, cloudProvider)
	if err != nil {
		log.Error(err, "❌ Failed to get secret value from provider", "provider", cloudProvider)
		return ctrl.Result{}, err
	}

	// ✅ Fetch the existing Kubernetes Secret
	var targetSecret corev1.Secret
	err = r.Client.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: targetSecretName}, &targetSecret)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("🔍 Target secret is missing, recreating", "name", targetSecretName)
		} else {
			log.Error(err, "❌ Failed to get existing target Secret", "name", targetSecretName)
			return ctrl.Result{}, err
		}
	}

	// 🛑 Check if the secret was just updated by this controller
	if val, exists := r.updatedSecrets.Load(targetSecretName); exists {
		log.Info("🔍 Checking self-triggered update",
			"secret", targetSecretName,
			"storedResourceVersion", val,
			"currentResourceVersion", targetSecret.ResourceVersion)

		if resourceVersion, ok := val.(string); ok && resourceVersion == targetSecret.ResourceVersion {
			log.Info("🚫 Ignoring self-triggered update", "secret", targetSecretName)
			return ctrl.Result{}, nil
		}
	}

	// ✅ Compare current secret with expected secret
	if !utils.CompareSecretData(targetSecret.Data, secretValue) {
		log.Info("⚠️ Secret modified manually! Restoring original version...", "name", targetSecretName)

		// Restore the correct secret
		err := utils.CreateOrUpdateK8sSecret(ctx, r.Client, req.Namespace, targetSecretName, secretValue, &r.updatedSecrets)
		if err != nil {
			log.Error(err, "❌ Failed to restore Kubernetes secret", "name", targetSecretName)
			return ctrl.Result{}, err
		}
	} else {
		log.Info("✅ Secret is up-to-date, no changes needed", "name", targetSecretName)
	}

	// Rollout the Deployments that reference the secret
	err = utils.RolloutDeploymentsSecretUpdate(ctx, r.Client, targetSecretName, req.NamespacedName.Namespace)
	if err != nil {
		log.Error(err, "❌ Failed to rollout deployments", "secretName", targetSecretName)
		return ctrl.Result{}, err
	}

	// Mask secrets before logging
	log.Info("🔐 Secret processed successfully", "name", targetSecretName, "maskedValue", utils.MaskSecretData(secretValue))

	return ctrl.Result{}, nil
}

// ✅ Move Index Creation to a Separate Function
func (r *EventDrivenSecretReconciler) createFieldIndexes(mgr ctrl.Manager, ctx context.Context) error {
	log := ctrl.Log.WithName("IndexSetup")

	// Index for Deployment annotations
	if err := mgr.GetFieldIndexer().IndexField(ctx, &appsv1.Deployment{}, "metadata.annotations.eventsecrets",
		func(obj client.Object) []string {
			deployment := obj.(*appsv1.Deployment)
			secretsAnnotation, exists := deployment.Annotations["eventsecrets"]
			if !exists {
				return nil
			}

			var secretList []string
			if err := json.Unmarshal([]byte(secretsAnnotation), &secretList); err != nil {
				return nil
			}
			return secretList
		},
	); err != nil {
		return fmt.Errorf("❌ Failed to create index for metadata.annotations.eventsecrets: %w", err)
	}

	// Index for `spec.secretPath`
	if err := mgr.GetFieldIndexer().IndexField(ctx, &secretsv1alpha1.EventDrivenSecret{}, "spec.secretPath",
		func(obj client.Object) []string {
			secret := obj.(*secretsv1alpha1.EventDrivenSecret)
			if secret.Spec.SecretPath == "" {
				return nil
			}
			return []string{secret.Spec.SecretPath}
		},
	); err != nil {
		return fmt.Errorf("❌ Failed to create index for spec.secretPath: %w", err)
	}

	// Index for `spec.targetSecretName`
	if err := mgr.GetFieldIndexer().IndexField(ctx, &secretsv1alpha1.EventDrivenSecret{}, "spec.targetSecretName",
		func(obj client.Object) []string {
			eds := obj.(*secretsv1alpha1.EventDrivenSecret)
			if eds.Spec.TargetSecretName == "" {
				return nil
			}
			return []string{eds.Spec.TargetSecretName}
		},
	); err != nil {
		return fmt.Errorf("❌ Failed to create index for spec.targetSecretName: %w", err)
	}

	log.Info("✅ Indexing completed successfully")
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EventDrivenSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {

	log := ctrl.Log.WithName("SetupWithManager")

	// ✅ Store manager instance in struct for later use
	r.mgr = mgr

	// ✅ Use Background Context
	ctx := context.Background()

	// 🔄 Step 1: Create necessary field indexes FIRST!
	if err := r.createFieldIndexes(mgr, ctx); err != nil {
		log.Error(err, "❌ Failed to create indexes")
		return err
	}

	// ✅ Step 2: Wait for cache to sync before listing resources
	go func() {
		<-mgr.Elected()                      // Ensures leader election (if enabled)
		mgr.GetCache().WaitForCacheSync(ctx) // Waits for cache sync

		log.Info("✅ Cache synced. Fetching existing EventDrivenSecrets...")

		var eventDrivenSecrets secretsv1alpha1.EventDrivenSecretList
		if err := mgr.GetClient().List(ctx, &eventDrivenSecrets); err != nil {
			log.Error(err, "❌ Failed to list existing EventDrivenSecrets after cache sync")
			return
		}

		for _, eds := range eventDrivenSecrets.Items {
			cloudProvider := eds.Spec.CloudProvider

			// **Check if listener is already running before starting**
			if _, exists := r.activeProviders.Load(cloudProvider); !exists {
				log.Info("🔄 Starting listener for provider", "provider", cloudProvider)
				r.activeProviders.Store(cloudProvider, true)

				go func() {
					providers.StartListening(ctx, cloudProvider, mgr.GetClient(), &r.updatedSecrets)
					log.Info("✅ Listener started for provider", "provider", cloudProvider)
				}()
			}
		}
	}()

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

				// Retrieve last known resourceVersion
				if lastVersion, exists := r.updatedSecrets.Load(secret.Name); exists {
					if lastVersion == secret.ResourceVersion {
						ctrl.Log.Info("🔄 Skipping self-triggered reconciliation", "secret", secret.Name, "resourceVersion", secret.ResourceVersion)
						return nil
					}
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
