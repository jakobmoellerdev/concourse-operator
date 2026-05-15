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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

var _ = Describe("ConcoursePipeline Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-pipeline"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: "default"}
		pipeline := &concoursev1alpha1.ConcoursePipeline{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ConcoursePipeline")
			err := k8sClient.Get(ctx, typeNamespacedName, pipeline)
			if err != nil && errors.IsNotFound(err) {
				resource := &concoursev1alpha1.ConcoursePipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: concoursev1alpha1.ConcoursePipelineSpec{
						TeamRef: concoursev1alpha1.LocalObjectReference{Name: "test-team"},
						Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &concoursev1alpha1.ConcoursePipeline{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance ConcoursePipeline")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile without error when team is missing", func() {
			By("Reconciling the created resource")
			reconciler := &ConcoursePipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should add the pipeline finalizer on first reconcile", func() {
			By("Creating a fresh pipeline for this test")
			finalizerPipeline := &concoursev1alpha1.ConcoursePipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline-finalizer", Namespace: "default"},
				Spec: concoursev1alpha1.ConcoursePipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: "test-team"},
					Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
				},
			}
			Expect(k8sClient.Create(ctx, finalizerPipeline)).To(Succeed())
			DeferCleanup(func() {
				latest := &concoursev1alpha1.ConcoursePipeline{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-pipeline-finalizer", Namespace: "default"}, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, pipelineFinalizer)
					_ = k8sClient.Update(ctx, latest)
					_ = k8sClient.Delete(ctx, latest)
				}
			})

			reconciler := &ConcoursePipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			finalizerNSN := types.NamespacedName{Name: "test-pipeline-finalizer", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: finalizerNSN})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that the finalizer was added")
			fetched := &concoursev1alpha1.ConcoursePipeline{}
			Expect(k8sClient.Get(ctx, finalizerNSN, fetched)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fetched, pipelineFinalizer)).To(BeTrue())
		})

		It("should set Ready=False when referenced team is not ready", func() {
			By("Creating a team with no Ready condition")
			notReadyTeam := &concoursev1alpha1.ConcourseTeam{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-team-not-ready", Namespace: "default"},
				Spec: concoursev1alpha1.ConcourseTeamSpec{
					InstanceRef: concoursev1alpha1.LocalObjectReference{Name: "some-instance"},
				},
			}
			Expect(k8sClient.Create(ctx, notReadyTeam)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, notReadyTeam) })

			By("Creating a pipeline referencing the not-ready team")
			pl := &concoursev1alpha1.ConcoursePipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-team-check", Namespace: "default"},
				Spec: concoursev1alpha1.ConcoursePipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: "pipeline-team-not-ready"},
					Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
				},
			}
			Expect(k8sClient.Create(ctx, pl)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pl) })

			reconciler := &ConcoursePipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "pipeline-team-check", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying Ready=False is set")
			fetched := &concoursev1alpha1.ConcoursePipeline{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "pipeline-team-check", Namespace: "default"}, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should set Ready=False with reason ConfigLoadFailed when ConfigMap does not exist", func() {
			By("Creating a ready instance and team chain")
			inst := makeReadyInstance(ctx, "pipeline-cm-instance")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "pipeline-cm-team", "pipeline-cm-instance")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })

			By("Creating a pipeline using a non-existent ConfigMap")
			pl := &concoursev1alpha1.ConcoursePipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-missing-cm", Namespace: "default"},
				Spec: concoursev1alpha1.ConcoursePipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: "pipeline-cm-team"},
					Config: concoursev1alpha1.PipelineConfig{
						ConfigMapRef: &concoursev1alpha1.ConfigMapKeyRef{Name: "missing-cm", Key: "pipeline.yml"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pl)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pl) })

			reconciler := &ConcoursePipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "pipeline-missing-cm", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying ConfigLoadFailed condition")
			fetched := &concoursev1alpha1.ConcoursePipeline{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "pipeline-missing-cm", Namespace: "default"}, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("ConfigLoadFailed"))
		})

		It("should set Ready=False with reason ConfigLoadFailed when ConfigMap key is missing", func() {
			By("Creating a ready instance and team chain")
			inst := makeReadyInstance(ctx, "pipeline-cm-key-instance")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "pipeline-cm-key-team", "pipeline-cm-key-instance")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })

			By("Creating a ConfigMap without the expected key")
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-config-cm", Namespace: "default"},
				Data:       map[string]string{"other-key": "value"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cm) })

			By("Creating a pipeline referencing a missing key")
			pl := &concoursev1alpha1.ConcoursePipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-bad-key", Namespace: "default"},
				Spec: concoursev1alpha1.ConcoursePipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: "pipeline-cm-key-team"},
					Config: concoursev1alpha1.PipelineConfig{
						ConfigMapRef: &concoursev1alpha1.ConfigMapKeyRef{Name: "pipeline-config-cm", Key: "pipeline.yml"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pl)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pl) })

			reconciler := &ConcoursePipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "pipeline-bad-key", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying ConfigLoadFailed condition")
			fetched := &concoursev1alpha1.ConcoursePipeline{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "pipeline-bad-key", Namespace: "default"}, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("ConfigLoadFailed"))
		})
	})
})
