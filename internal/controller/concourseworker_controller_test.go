/*
Copyright 2026.

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

var _ = Describe("ConcourseWorker Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-worker"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: "default"}
		worker := &concoursev1alpha1.ConcourseWorker{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ConcourseWorker")
			err := k8sClient.Get(ctx, typeNamespacedName, worker)
			if err != nil && errors.IsNotFound(err) {
				resource := &concoursev1alpha1.ConcourseWorker{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: concoursev1alpha1.ConcourseWorkerSpec{
						InstanceRef:  concoursev1alpha1.LocalObjectReference{Name: "test-instance"},
						WorkerName:   "worker-1",
						DesiredState: concoursev1alpha1.WorkerDesiredStateActive,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &concoursev1alpha1.ConcourseWorker{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance ConcourseWorker")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile without error when instance is missing", func() {
			By("Reconciling the created resource")
			reconciler := &ConcourseWorkerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			// Instance doesn't exist: Get returns NotFound → IgnoreNotFound → no error, requeue.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should set Ready=False with InstanceNotReady reason when instance exists but is not ready", func() {
			By("Creating an instance without Ready condition")
			notReadyInst := &concoursev1alpha1.ConcourseInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-instance-not-ready", Namespace: "default"},
				Spec:       concoursev1alpha1.ConcourseInstanceSpec{URL: "https://ci.example.com"},
			}
			Expect(k8sClient.Create(ctx, notReadyInst)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, notReadyInst) })

			By("Creating a worker referencing the not-ready instance")
			notReadyWorker := &concoursev1alpha1.ConcourseWorker{
				ObjectMeta: metav1.ObjectMeta{Name: "worker-instance-check", Namespace: "default"},
				Spec: concoursev1alpha1.ConcourseWorkerSpec{
					InstanceRef:  concoursev1alpha1.LocalObjectReference{Name: "worker-instance-not-ready"},
					WorkerName:   "worker-1",
					DesiredState: concoursev1alpha1.WorkerDesiredStateActive,
				},
			}
			Expect(k8sClient.Create(ctx, notReadyWorker)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, notReadyWorker) })

			reconciler := &ConcourseWorkerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "worker-instance-check", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying Ready=False with InstanceNotReady reason")
			fetched := &concoursev1alpha1.ConcourseWorker{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "worker-instance-check", Namespace: "default"}, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("InstanceNotReady"))
		})
	})
})
