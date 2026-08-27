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
	"fmt"

	"github.com/concourse/concourse/atc"
	goconcourse "github.com/concourse/concourse/go-concourse/concourse"
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

var _ = Describe("Pipeline Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-pipeline"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: "default"}
		pipeline := &concoursev1alpha1.Pipeline{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Pipeline")
			err := k8sClient.Get(ctx, typeNamespacedName, pipeline)
			if err != nil && errors.IsNotFound(err) {
				resource := &concoursev1alpha1.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: concoursev1alpha1.PipelineSpec{
						TeamRef: concoursev1alpha1.LocalObjectReference{Name: "test-team"},
						Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &concoursev1alpha1.Pipeline{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance Pipeline")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile without error when team is missing", func() {
			By("Reconciling the created resource")
			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should add the pipeline finalizer on first reconcile", func() {
			By("Creating a fresh pipeline for this test")
			finalizerPipeline := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline-finalizer", Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: "test-team"},
					Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
				},
			}
			Expect(k8sClient.Create(ctx, finalizerPipeline)).To(Succeed())
			DeferCleanup(func() {
				latest := &concoursev1alpha1.Pipeline{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-pipeline-finalizer", Namespace: "default"}, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, pipelineFinalizer)
					_ = k8sClient.Update(ctx, latest)
					_ = k8sClient.Delete(ctx, latest)
				}
			})

			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			finalizerNSN := types.NamespacedName{Name: "test-pipeline-finalizer", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: finalizerNSN})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that the finalizer was added")
			fetched := &concoursev1alpha1.Pipeline{}
			Expect(k8sClient.Get(ctx, finalizerNSN, fetched)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(fetched, pipelineFinalizer)).To(BeTrue())
		})

		It("should set Ready=False when referenced team is not ready", func() {
			By("Creating a team with no Ready condition")
			notReadyTeam := &concoursev1alpha1.Team{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-team-not-ready", Namespace: "default"},
				Spec: concoursev1alpha1.TeamSpec{
					InstanceRef: concoursev1alpha1.LocalObjectReference{Name: "some-instance"},
				},
			}
			Expect(k8sClient.Create(ctx, notReadyTeam)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, notReadyTeam) })

			By("Creating a pipeline referencing the not-ready team")
			pl := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-team-check", Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: "pipeline-team-not-ready"},
					Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
				},
			}
			Expect(k8sClient.Create(ctx, pl)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pl) })

			reconciler := &PipelineReconciler{
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
			fetched := &concoursev1alpha1.Pipeline{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "pipeline-team-check", Namespace: "default"}, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should set Ready=False with reason ConfigLoadFailed when ConfigMap does not exist", func() {
			By("Creating a ready instance and team chain")
			cache := concourse.NewCache()
			fake := &fakeClient{team: &fakeTeam{name: "main"}}
			inst := makeReadyInstanceWithFakeClient(ctx, "pipeline-cm-instance", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "pipeline-cm-team", "pipeline-cm-instance")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })

			By("Creating a pipeline using a non-existent ConfigMap")
			pl := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-missing-cm", Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: "pipeline-cm-team"},
					Config: concoursev1alpha1.PipelineConfig{
						ConfigMapRef: &concoursev1alpha1.ConfigMapKeyRef{Name: "missing-cm", Key: "pipeline.yml"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pl)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pl) })

			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "pipeline-missing-cm", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying ConfigLoadFailed condition")
			fetched := &concoursev1alpha1.Pipeline{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "pipeline-missing-cm", Namespace: "default"}, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("ConfigLoadFailed"))
		})

		It("should set Ready=False with reason ConfigLoadFailed when ConfigMap key is missing", func() {
			By("Creating a ready instance and team chain")
			cache := concourse.NewCache()
			fake := &fakeClient{team: &fakeTeam{name: "main"}}
			inst := makeReadyInstanceWithFakeClient(ctx, "pipeline-cm-key-instance", cache, fake)
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
			pl := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-bad-key", Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: "pipeline-cm-key-team"},
					Config: concoursev1alpha1.PipelineConfig{
						ConfigMapRef: &concoursev1alpha1.ConfigMapKeyRef{Name: "pipeline-config-cm", Key: "pipeline.yml"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pl)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pl) })

			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "pipeline-bad-key", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying ConfigLoadFailed condition")
			fetched := &concoursev1alpha1.Pipeline{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "pipeline-bad-key", Namespace: "default"}, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("ConfigLoadFailed"))
		})
	})

	Context("When Concourse API responds successfully", func() {
		ctx := context.Background()

		It("should call CreateOrUpdatePipelineConfig with inline config and set Ready=True", func() {
			By("Setting up a fake client that records pipeline API calls")
			cache := concourse.NewCache()
			var capturedConfig []byte
			pauseCalled, unpauseCalled, hideCalled := false, false, false
			fake := &fakeClient{
				team: &fakeTeam{
					name: "main",
					createOrUpdatePipelineConfigFn: func(ref atc.PipelineRef, _ string, cfg []byte, _ bool) (bool, bool, []goconcourse.ConfigWarning, error) {
						capturedConfig = cfg
						return true, false, nil, nil
					},
					pausePipelineFn:   func(_ atc.PipelineRef) (bool, error) { pauseCalled = true; return true, nil },
					unpausePipelineFn: func(_ atc.PipelineRef) (bool, error) { unpauseCalled = true; return true, nil },
					hidePipelineFn:    func(_ atc.PipelineRef) (bool, error) { hideCalled = true; return true, nil },
					pipelineFn:        func(_ atc.PipelineRef) (atc.Pipeline, bool, error) { return atc.Pipeline{ID: 7}, true, nil },
				},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "pipe-happy-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })

			team := makeReadyTeam(ctx, "pipe-happy-team", inst.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })

			pl := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "pipe-happy", Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: team.Name},
					Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
					Paused:  false,
					Exposed: false,
				},
			}
			Expect(k8sClient.Create(ctx, pl)).To(Succeed())
			DeferCleanup(func() {
				latest := &concoursev1alpha1.Pipeline{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: "pipe-happy", Namespace: "default"}, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, pipelineFinalizer)
					_ = k8sClient.Update(ctx, latest)
					_ = k8sClient.Delete(ctx, latest)
				}
			})

			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "pipe-happy", Namespace: "default"}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying API was called with the correct config")
			Expect(capturedConfig).To(Equal([]byte("jobs: []")))
			Expect(unpauseCalled).To(BeTrue(), "expected UnpausePipeline to be called")
			Expect(hideCalled).To(BeTrue(), "expected HidePipeline to be called")
			Expect(pauseCalled).To(BeFalse(), "PausePipeline should not be called when paused=false")

			By("Verifying status fields and Ready=True")
			fetched := &concoursev1alpha1.Pipeline{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.ConfigHash).NotTo(BeEmpty())
			Expect(fetched.Status.PipelineID).NotTo(BeNil())
			Expect(*fetched.Status.PipelineID).To(Equal(int32(7)))
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should not call CreateOrUpdatePipelineConfig when config hash is unchanged", func() {
			By("Setting up a fake client")
			cache := concourse.NewCache()
			callCount := 0
			fake := &fakeClient{
				team: &fakeTeam{
					name: "main",
					createOrUpdatePipelineConfigFn: func(_ atc.PipelineRef, _ string, _ []byte, _ bool) (bool, bool, []goconcourse.ConfigWarning, error) {
						callCount++
						return false, false, nil, nil
					},
					pipelineFn: func(_ atc.PipelineRef) (atc.Pipeline, bool, error) { return atc.Pipeline{ID: 5}, true, nil },
				},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "pipe-noupdate-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "pipe-noupdate-team", inst.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })

			pl := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "pipe-noupdate", Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: team.Name},
					Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
				},
			}
			Expect(k8sClient.Create(ctx, pl)).To(Succeed())
			DeferCleanup(func() {
				latest := &concoursev1alpha1.Pipeline{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: "pipe-noupdate", Namespace: "default"}, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, pipelineFinalizer)
					_ = k8sClient.Update(ctx, latest)
					_ = k8sClient.Delete(ctx, latest)
				}
			})

			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "pipe-noupdate", Namespace: "default"}

			By("First reconcile creates pipeline")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(callCount).To(Equal(1))

			By("Second reconcile with same config hash skips CreateOrUpdate")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(callCount).To(Equal(1), "CreateOrUpdatePipelineConfig should not be called again when hash unchanged")
		})

		It("should call PausePipeline when spec.paused=true", func() {
			By("Setting up a fake client")
			cache := concourse.NewCache()
			pauseCalled := false
			fake := &fakeClient{
				team: &fakeTeam{
					name: "main",
					createOrUpdatePipelineConfigFn: func(_ atc.PipelineRef, _ string, _ []byte, _ bool) (bool, bool, []goconcourse.ConfigWarning, error) {
						return true, false, nil, nil
					},
					pausePipelineFn: func(_ atc.PipelineRef) (bool, error) { pauseCalled = true; return true, nil },
					pipelineFn: func(_ atc.PipelineRef) (atc.Pipeline, bool, error) {
						return atc.Pipeline{ID: 9, Paused: true}, true, nil
					},
				},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "pipe-pause-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "pipe-pause-team", inst.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })

			pl := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "pipe-pause", Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: team.Name},
					Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
					Paused:  true,
				},
			}
			Expect(k8sClient.Create(ctx, pl)).To(Succeed())
			DeferCleanup(func() {
				latest := &concoursev1alpha1.Pipeline{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: "pipe-pause", Namespace: "default"}, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, pipelineFinalizer)
					_ = k8sClient.Update(ctx, latest)
					_ = k8sClient.Delete(ctx, latest)
				}
			})

			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "pipe-pause", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(pauseCalled).To(BeTrue(), "expected PausePipeline to be called")

			fetched := &concoursev1alpha1.Pipeline{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.Paused).NotTo(BeNil())
			Expect(*fetched.Status.Paused).To(BeTrue())
		})

		It("should set Ready=False/SetPipelineFailed when CreateOrUpdatePipelineConfig returns error", func() {
			By("Setting up a fake client that returns an error")
			cache := concourse.NewCache()
			fake := &fakeClient{
				team: &fakeTeam{
					name: "main",
					createOrUpdatePipelineConfigFn: func(_ atc.PipelineRef, _ string, _ []byte, _ bool) (bool, bool, []goconcourse.ConfigWarning, error) {
						return false, false, nil, fmt.Errorf("concourse api error")
					},
				},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "pipe-err-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "pipe-err-team", inst.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })

			pl := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "pipe-err", Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: team.Name},
					Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
				},
			}
			Expect(k8sClient.Create(ctx, pl)).To(Succeed())
			DeferCleanup(func() {
				latest := &concoursev1alpha1.Pipeline{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: "pipe-err", Namespace: "default"}, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, pipelineFinalizer)
					_ = k8sClient.Update(ctx, latest)
					_ = k8sClient.Delete(ctx, latest)
				}
			})

			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "pipe-err", Namespace: "default"}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			fetched := &concoursev1alpha1.Pipeline{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("SetPipelineFailed"))
		})

		It("should call DeletePipeline on finalization", func() {
			By("Setting up a fake client that records DeletePipeline calls")
			cache := concourse.NewCache()
			deleteCalled := false
			fake := &fakeClient{
				team: &fakeTeam{
					name: "main",
					createOrUpdatePipelineConfigFn: func(_ atc.PipelineRef, _ string, _ []byte, _ bool) (bool, bool, []goconcourse.ConfigWarning, error) {
						return true, false, nil, nil
					},
					deletePipelineFn: func(_ atc.PipelineRef) (bool, error) { deleteCalled = true; return true, nil },
					pipelineFn:       func(_ atc.PipelineRef) (atc.Pipeline, bool, error) { return atc.Pipeline{ID: 3}, true, nil },
				},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst := makeReadyInstanceWithFakeClient(ctx, "pipe-del-inst", cache, fake)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, inst) })
			team := makeReadyTeam(ctx, "pipe-del-team", inst.Name)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, team) })

			pl := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "pipe-del", Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: team.Name},
					Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
				},
			}
			Expect(k8sClient.Create(ctx, pl)).To(Succeed())

			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "pipe-del", Namespace: "default"}

			By("First reconcile adds finalizer")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())

			By("Deleting triggers DeletePipeline")
			fetched := &concoursev1alpha1.Pipeline{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(k8sClient.Delete(ctx, fetched)).To(Succeed())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(deleteCalled).To(BeTrue(), "expected DeletePipeline to be called during finalization")
		})
	})
})
