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

var _ = Describe("ConcourseBuild Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-build"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: "default"}
		build := &concoursev1alpha1.ConcourseBuild{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ConcourseBuild")
			err := k8sClient.Get(ctx, typeNamespacedName, build)
			if err != nil && errors.IsNotFound(err) {
				resource := &concoursev1alpha1.ConcourseBuild{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: concoursev1alpha1.ConcourseBuildSpec{
						JobRef: &concoursev1alpha1.LocalObjectReference{Name: "test-job"},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &concoursev1alpha1.ConcourseBuild{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance ConcourseBuild")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile without error when job is missing", func() {
			By("Reconciling the created resource")
			reconciler := &ConcourseBuildReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			// Job doesn't exist → IgnoreNotFound → requeue, no error.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should set Ready=False with NoJobRef reason when jobRef is nil", func() {
			By("Creating a build with no jobRef")
			noJobRefBuild := &concoursev1alpha1.ConcourseBuild{
				ObjectMeta: metav1.ObjectMeta{Name: "build-no-jobref", Namespace: "default"},
				Spec:       concoursev1alpha1.ConcourseBuildSpec{},
			}
			Expect(k8sClient.Create(ctx, noJobRefBuild)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, noJobRefBuild) })

			reconciler := &ConcourseBuildReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "build-no-jobref", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			// No jobRef: no requeue.
			Expect(result.RequeueAfter).To(BeZero())

			By("Verifying Ready=False with NoJobRef reason")
			fetched := &concoursev1alpha1.ConcourseBuild{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "build-no-jobref", Namespace: "default"}, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("NoJobRef"))
		})
	})

	Context("When build is in a terminal state", func() {
		ctx := context.Background()

		DescribeTable("should return no-op without status change",
			func(terminalStatus concoursev1alpha1.BuildStatus) {
				buildName := "build-terminal-" + string(terminalStatus)
				terminalBuild := &concoursev1alpha1.ConcourseBuild{
					ObjectMeta: metav1.ObjectMeta{Name: buildName, Namespace: "default"},
					Spec: concoursev1alpha1.ConcourseBuildSpec{
						JobRef: &concoursev1alpha1.LocalObjectReference{Name: "some-job"},
					},
				}
				Expect(k8sClient.Create(ctx, terminalBuild)).To(Succeed())
				DeferCleanup(func() { _ = k8sClient.Delete(ctx, terminalBuild) })

				terminalBuild.Status.ConcourseStatus = terminalStatus
				Expect(k8sClient.Status().Update(ctx, terminalBuild)).To(Succeed())

				reconciler := &ConcourseBuildReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
					Cache:  concourse.NewCache(),
				}
				result, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: buildName, Namespace: "default"},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero(), "terminal build should not requeue")

				// Status must remain terminal.
				fetched := &concoursev1alpha1.ConcourseBuild{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: buildName, Namespace: "default"}, fetched)).To(Succeed())
				Expect(fetched.Status.ConcourseStatus).To(Equal(terminalStatus))
			},
			Entry("succeeded", concoursev1alpha1.BuildStatusSucceeded),
			Entry("failed", concoursev1alpha1.BuildStatusFailed),
			Entry("errored", concoursev1alpha1.BuildStatusErrored),
			Entry("aborted", concoursev1alpha1.BuildStatusAborted),
		)
	})

	Context("When build chain is not ready", func() {
		ctx := context.Background()

		It("should set Ready=False with ChainNotReady reason when pipeline is not ready", func() {
			By("Creating chain: instance, team, pipeline (not ready), job")
			inst := makeReadyInstance(ctx, "build-chain-instance")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "build-chain-team", "build-chain-instance")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })

			notReadyPipeline := &concoursev1alpha1.ConcoursePipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "build-chain-pipeline", Namespace: "default"},
				Spec: concoursev1alpha1.ConcoursePipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: "build-chain-team"},
					Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
				},
			}
			Expect(k8sClient.Create(ctx, notReadyPipeline)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, notReadyPipeline) })

			chainJob := &concoursev1alpha1.ConcourseJob{
				ObjectMeta: metav1.ObjectMeta{Name: "build-chain-job", Namespace: "default"},
				Spec: concoursev1alpha1.ConcourseJobSpec{
					PipelineRef: concoursev1alpha1.LocalObjectReference{Name: "build-chain-pipeline"},
					JobName:     "build",
				},
			}
			Expect(k8sClient.Create(ctx, chainJob)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, chainJob) })

			chainBuild := &concoursev1alpha1.ConcourseBuild{
				ObjectMeta: metav1.ObjectMeta{Name: "build-chain-build", Namespace: "default"},
				Spec: concoursev1alpha1.ConcourseBuildSpec{
					JobRef: &concoursev1alpha1.LocalObjectReference{Name: "build-chain-job"},
				},
			}
			Expect(k8sClient.Create(ctx, chainBuild)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, chainBuild) })

			reconciler := &ConcourseBuildReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "build-chain-build", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying Ready=False with ChainNotReady reason")
			fetched := &concoursev1alpha1.ConcourseBuild{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "build-chain-build", Namespace: "default"}, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("ChainNotReady"))
		})
	})
})
