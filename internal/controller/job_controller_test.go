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

	"github.com/concourse/concourse/atc"
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

var _ = Describe("Job Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-job"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: "default"}
		job := &concoursev1alpha1.Job{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Job")
			err := k8sClient.Get(ctx, typeNamespacedName, job)
			if err != nil && errors.IsNotFound(err) {
				resource := &concoursev1alpha1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: concoursev1alpha1.JobSpec{
						PipelineRef: concoursev1alpha1.LocalObjectReference{Name: "test-pipeline"},
						JobName:     "build",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &concoursev1alpha1.Job{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance Job")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile without error when pipeline is missing", func() {
			By("Reconciling the created resource")
			reconciler := &JobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should set Ready=False when referenced pipeline is not ready", func() {
			By("Creating a pipeline with no Ready condition")
			notReadyPipeline := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "job-pipeline-not-ready", Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: "some-team"},
					Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
				},
			}
			Expect(k8sClient.Create(ctx, notReadyPipeline)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, notReadyPipeline) })

			By("Creating a job referencing the not-ready pipeline")
			notReadyJob := &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "job-pipeline-check", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef: concoursev1alpha1.LocalObjectReference{Name: "job-pipeline-not-ready"},
					JobName:     "build",
				},
			}
			Expect(k8sClient.Create(ctx, notReadyJob)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, notReadyJob) })

			reconciler := &JobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "job-pipeline-check", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying Ready=False is set")
			fetched := &concoursev1alpha1.Job{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "job-pipeline-check", Namespace: "default"}, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should create a Build CR when TriggerBuild=true and chain is ready", func() {
			By("Setting up a ready instance/team/pipeline chain")
			inst := makeReadyInstance(ctx, "job-trigger-instance")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "job-trigger-team", "job-trigger-instance")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })
			pipeline := makeReadyPipeline(ctx, "job-trigger-pipeline", "job-trigger-team")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pipeline) })

			By("Creating a job with TriggerBuild=true")
			triggerJob := &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "job-with-trigger", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef:  concoursev1alpha1.LocalObjectReference{Name: "job-trigger-pipeline"},
					JobName:      "build",
					TriggerBuild: true,
				},
			}
			Expect(k8sClient.Create(ctx, triggerJob)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, triggerJob)
				buildList := &concoursev1alpha1.BuildList{}
				_ = k8sClient.List(ctx, buildList)
				for i := range buildList.Items {
					_ = k8sClient.Delete(ctx, &buildList.Items[i])
				}
			})

			// Fresh resource has Generation=1, ObservedGeneration=0, so trigger fires.
			reconciler := &JobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "job-with-trigger", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying a Build CR was created")
			buildList := &concoursev1alpha1.BuildList{}
			Expect(k8sClient.List(ctx, buildList)).To(Succeed())
			found := false
			for _, b := range buildList.Items {
				if b.Spec.JobRef != nil && b.Spec.JobRef.Name == "job-with-trigger" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected a Build CR referencing job-with-trigger")
		})

		It("should NOT create a Build CR when TriggerBuild=false", func() {
			By("Setting up a ready instance/team/pipeline chain")
			inst := makeReadyInstance(ctx, "job-notrigger-instance")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "job-notrigger-team", "job-notrigger-instance")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })
			pipeline := makeReadyPipeline(ctx, "job-notrigger-pipeline", "job-notrigger-team")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pipeline) })

			By("Creating a job with TriggerBuild=false")
			noTriggerJob := &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "job-without-trigger", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef:  concoursev1alpha1.LocalObjectReference{Name: "job-notrigger-pipeline"},
					JobName:      "build",
					TriggerBuild: false,
				},
			}
			Expect(k8sClient.Create(ctx, noTriggerJob)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, noTriggerJob) })

			reconciler := &JobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "job-without-trigger", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying no Build CR was created for this job")
			buildList := &concoursev1alpha1.BuildList{}
			Expect(k8sClient.List(ctx, buildList)).To(Succeed())
			for _, b := range buildList.Items {
				if b.Spec.JobRef != nil {
					Expect(b.Spec.JobRef.Name).NotTo(Equal("job-without-trigger"))
				}
			}
		})
	})

	Context("When Concourse API responds successfully", func() {
		ctx := context.Background()

		It("should NOT trigger a second build when ObservedGeneration already matches Generation", func() {
			By("Setting up a ready chain with fake client")
			cache := concourse.NewCache()
			fake := &fakeClient{
				team: &fakeTeam{
					name:         "jt-team",
					unpauseJobFn: func(_ atc.PipelineRef, _ string) (bool, error) { return true, nil },
				},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "jt-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "jt-team", inst.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })
			pipeline := makeReadyPipeline(ctx, "jt-pipeline", team.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pipeline) })

			triggerJob := &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "job-idempotent", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef:  concoursev1alpha1.LocalObjectReference{Name: pipeline.Name},
					JobName:      "deploy",
					TriggerBuild: true,
				},
			}
			Expect(k8sClient.Create(ctx, triggerJob)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, triggerJob)
				buildList := &concoursev1alpha1.BuildList{}
				_ = k8sClient.List(ctx, buildList)
				for i := range buildList.Items {
					_ = k8sClient.Delete(ctx, &buildList.Items[i])
				}
			})

			reconciler := &JobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "job-idempotent", Namespace: "default"}

			By("First reconcile creates a build (Generation=1, ObservedGeneration=0)")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())

			buildListAfterFirst := &concoursev1alpha1.BuildList{}
			Expect(k8sClient.List(ctx, buildListAfterFirst)).To(Succeed())
			countAfterFirst := 0
			for _, b := range buildListAfterFirst.Items {
				if b.Spec.JobRef != nil && b.Spec.JobRef.Name == "job-idempotent" {
					countAfterFirst++
				}
			}
			Expect(countAfterFirst).To(Equal(1), "expected exactly 1 build after first reconcile")

			By("Second reconcile with same generation should NOT create another build")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())

			buildListAfterSecond := &concoursev1alpha1.BuildList{}
			Expect(k8sClient.List(ctx, buildListAfterSecond)).To(Succeed())
			countAfterSecond := 0
			for _, b := range buildListAfterSecond.Items {
				if b.Spec.JobRef != nil && b.Spec.JobRef.Name == "job-idempotent" {
					countAfterSecond++
				}
			}
			Expect(countAfterSecond).To(Equal(1), "second reconcile should not create a duplicate build")
		})

		It("should call PauseJob when spec.paused=true and set status.Paused=true", func() {
			cache := concourse.NewCache()
			pauseCalled := false
			fake := &fakeClient{
				team: &fakeTeam{
					name: "jp-team",
					pauseJobFn: func(_ atc.PipelineRef, _ string) (bool, error) {
						pauseCalled = true
						return true, nil
					},
				},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "jp-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "jp-team", inst.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })
			pipeline := makeReadyPipeline(ctx, "jp-pipeline", team.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pipeline) })

			pausedJob := &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "job-paused", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef: concoursev1alpha1.LocalObjectReference{Name: pipeline.Name},
					JobName:     "deploy",
					Paused:      true,
				},
			}
			Expect(k8sClient.Create(ctx, pausedJob)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pausedJob) })

			reconciler := &JobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "job-paused", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(pauseCalled).To(BeTrue(), "expected PauseJob to be called")

			fetched := &concoursev1alpha1.Job{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.Paused).To(BeTrue())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should call UnpauseJob when spec.paused=false and set status.Paused=false", func() {
			cache := concourse.NewCache()
			unpauseCalled := false
			fake := &fakeClient{
				team: &fakeTeam{
					name: "jup-team",
					unpauseJobFn: func(_ atc.PipelineRef, _ string) (bool, error) {
						unpauseCalled = true
						return true, nil
					},
				},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "jup-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "jup-team", inst.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })
			pipeline := makeReadyPipeline(ctx, "jup-pipeline", team.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pipeline) })

			activeJob := &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "job-unpaused", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef: concoursev1alpha1.LocalObjectReference{Name: pipeline.Name},
					JobName:     "deploy",
					Paused:      false,
				},
			}
			Expect(k8sClient.Create(ctx, activeJob)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, activeJob) })

			reconciler := &JobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "job-unpaused", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(unpauseCalled).To(BeTrue(), "expected UnpauseJob to be called")

			fetched := &concoursev1alpha1.Job{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.Paused).To(BeFalse())
		})
	})
})
