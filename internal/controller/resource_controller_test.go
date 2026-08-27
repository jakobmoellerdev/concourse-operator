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
	"time"

	"github.com/concourse/concourse/atc"
	goconcourse "github.com/concourse/concourse/go-concourse/concourse"
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

	Context("When Concourse API responds successfully", func() {
		ctx := context.Background()

		It("should call CheckResource when interval elapsed and update status.LastChecked", func() {
			By("Setting up a ready chain with fake client")
			cache := concourse.NewCache()
			checkCalled := false
			fake := &fakeClient{
				team: &fakeTeam{
					name: "res-team",
					checkResourceFn: func(_ atc.PipelineRef, _ string, _ atc.Version, _ bool) (atc.Build, bool, error) {
						checkCalled = true
						return atc.Build{ID: 1}, true, nil
					},
					resourceVersionsFn: func(_ atc.PipelineRef, _ string, _ goconcourse.Page, _ atc.Version) ([]atc.ResourceVersion, goconcourse.Pagination, bool, error) {
						return []atc.ResourceVersion{{Version: atc.Version{"ref": "abc123"}}}, goconcourse.Pagination{}, true, nil
					},
				},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "res-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "res-team", inst.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })
			pipeline := makeReadyPipeline(ctx, "res-pipeline", team.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pipeline) })

			interval := metav1.Duration{Duration: time.Millisecond}
			cr := &concoursev1alpha1.Resource{
				ObjectMeta: metav1.ObjectMeta{Name: "res-check", Namespace: "default"},
				Spec: concoursev1alpha1.ResourceSpec{
					PipelineRef:   concoursev1alpha1.LocalObjectReference{Name: pipeline.Name},
					ResourceName:  "my-git",
					CheckInterval: &interval,
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })

			reconciler := &ResourceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "res-check", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(checkCalled).To(BeTrue(), "expected CheckResource to be called")

			By("Verifying status.LastChecked is set and Ready=True")
			fetched := &concoursev1alpha1.Resource{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.LastChecked).NotTo(BeNil())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should populate status.LatestVersion from ResourceVersions", func() {
			cache := concourse.NewCache()
			fake := &fakeClient{
				team: &fakeTeam{
					name: "res2-team",
					resourceVersionsFn: func(_ atc.PipelineRef, _ string, _ goconcourse.Page, _ atc.Version) ([]atc.ResourceVersion, goconcourse.Pagination, bool, error) {
						return []atc.ResourceVersion{{Version: atc.Version{"ref": "deadbeef"}}}, goconcourse.Pagination{}, true, nil
					},
				},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "res2-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "res2-team", inst.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })
			pipeline := makeReadyPipeline(ctx, "res2-pipeline", team.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pipeline) })

			cr := &concoursev1alpha1.Resource{
				ObjectMeta: metav1.ObjectMeta{Name: "res-version", Namespace: "default"},
				Spec: concoursev1alpha1.ResourceSpec{
					PipelineRef:  concoursev1alpha1.LocalObjectReference{Name: pipeline.Name},
					ResourceName: "my-git",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })

			reconciler := &ResourceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "res-version", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())

			fetched := &concoursev1alpha1.Resource{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.LatestVersion).To(Equal(map[string]string{"ref": "deadbeef"}))
		})

		It("should pin a matching resource version", func() {
			cache := concourse.NewCache()
			pinCalled := false
			fake := &fakeClient{
				team: &fakeTeam{
					name: "res3-team",
					resourceFn: func(_ atc.PipelineRef, name string) (atc.Resource, bool, error) {
						return atc.Resource{Name: name}, true, nil
					},
					resourceVersionsFn: func(_ atc.PipelineRef, _ string, _ goconcourse.Page, _ atc.Version) ([]atc.ResourceVersion, goconcourse.Pagination, bool, error) {
						return []atc.ResourceVersion{{ID: 9, Version: atc.Version{"ref": "abc"}}}, goconcourse.Pagination{}, true, nil
					},
					pinResourceVersionFn: func(_ atc.PipelineRef, _ string, id int) (bool, error) {
						pinCalled = true
						Expect(id).To(Equal(9))
						return true, nil
					},
				},
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "res3-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "res3-team", inst.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })
			pipeline := makeReadyPipeline(ctx, "res3-pipeline", team.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pipeline) })

			cr := &concoursev1alpha1.Resource{
				ObjectMeta: metav1.ObjectMeta{Name: "res-pin", Namespace: "default"},
				Spec: concoursev1alpha1.ResourceSpec{
					PipelineRef:   concoursev1alpha1.LocalObjectReference{Name: pipeline.Name},
					ResourceName:  "my-git",
					PinnedVersion: map[string]string{"ref": "abc"},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })

			reconciler := &ResourceReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Cache: cache}
			nsn := types.NamespacedName{Name: "res-pin", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(pinCalled).To(BeTrue())

			fetched := &concoursev1alpha1.Resource{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.Pinned).NotTo(BeNil())
			Expect(*fetched.Status.Pinned).To(BeTrue())
			Expect(fetched.Status.PinnedVersionID).NotTo(BeNil())
			Expect(*fetched.Status.PinnedVersionID).To(Equal(int32(9)))
		})
	})
})
