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

var _ = Describe("Resource Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: "default"}
		concourseresource := &concoursev1alpha1.Resource{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Resource")
			err := k8sClient.Get(ctx, typeNamespacedName, concourseresource)
			if err != nil && errors.IsNotFound(err) {
				resource := &concoursev1alpha1.Resource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: concoursev1alpha1.ResourceSpec{
						PipelineRef:  concoursev1alpha1.LocalObjectReference{Name: "test-pipeline"},
						ResourceName: "my-resource",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &concoursev1alpha1.Resource{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance Resource")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile without error when pipeline is missing", func() {
			By("Reconciling the created resource")
			reconciler := &ResourceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			// Pipeline doesn't exist: IgnoreNotFound → reconciler returns no error.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should set Ready=False when pipeline exists but is not ready", func() {
			By("Creating a pipeline with no Ready condition")
			notReadyPipeline := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "resource-pipeline-not-ready", Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: "some-team"},
					Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
				},
			}
			Expect(k8sClient.Create(ctx, notReadyPipeline)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, notReadyPipeline) })

			By("Creating a Resource referencing it")
			cr := &concoursev1alpha1.Resource{
				ObjectMeta: metav1.ObjectMeta{Name: "resource-pipeline-check", Namespace: "default"},
				Spec: concoursev1alpha1.ResourceSpec{
					PipelineRef:  concoursev1alpha1.LocalObjectReference{Name: "resource-pipeline-not-ready"},
					ResourceName: "my-git",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })

			reconciler := &ResourceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "resource-pipeline-check", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying Ready=False is set with PipelineNotReady reason")
			fetched := &concoursev1alpha1.Resource{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "resource-pipeline-check", Namespace: "default"}, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("PipelineNotReady"))
		})
	})
})
