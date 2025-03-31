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
	"os"

	"k8s.io/utils/ptr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	secretsv1alpha1 "github.com/kkuyumjyan/k8s-event-driven-secrets/api/v1alpha1"
)

var _ = Describe("EventDrivenSecret Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		eventdrivensecret := &secretsv1alpha1.EventDrivenSecret{}

		BeforeEach(func() {
			// ✅ Inject env vars required by your controller
			_ = os.Setenv("AWS_REGION", "us-east-1")
			_ = os.Setenv("AWS_ACCESS_KEY_ID)", "test")
			_ = os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
			_ = os.Setenv("AWS_ENDPOINT", "http://localhost:4566")
			_ = os.Setenv("SQS_QUEUE_URL", "http://localhost:4566/000000000000/secret-updates")

			By("creating the custom resource for the Kind EventDrivenSecret")
			err := k8sClient.Get(ctx, typeNamespacedName, eventdrivensecret)
			if err != nil && errors.IsNotFound(err) {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment",
						Namespace: "default",
						Annotations: map[string]string{
							"eventdrivensecrets": `["my-secret-1"]`,
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
									Name:    "busybox",
									Image:   "busybox",
									Command: []string{"sleep", "3600"},
								}},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
				resource := &secretsv1alpha1.EventDrivenSecret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
					Spec: secretsv1alpha1.EventDrivenSecretSpec{
						// You're probably missing this:
						CloudProvider:    "aws",
						SecretPath:       "staging/common-secrets",
						TargetSecretName: "my-secret-1",
						CloudProviderOptions: secretsv1alpha1.CloudProviderOptions{
							Region: "us-east-1",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance EventDrivenSecret")
			resource := &secretsv1alpha1.EventDrivenSecret{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return
				}
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &EventDrivenSecretReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
