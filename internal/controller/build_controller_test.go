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
	concourseapi "github.com/concourse/concourse/go-concourse/concourse"
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

var _ = Describe("Build Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-build"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: "default"}
		build := &concoursev1alpha1.Build{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Build")
			err := k8sClient.Get(ctx, typeNamespacedName, build)
			if err != nil && errors.IsNotFound(err) {
				resource := &concoursev1alpha1.Build{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: concoursev1alpha1.BuildSpec{
						JobRef: &concoursev1alpha1.LocalObjectReference{Name: "test-job"},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &concoursev1alpha1.Build{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance Build")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile without error when job is missing", func() {
			By("Reconciling the created resource")
			reconciler := &BuildReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  concourse.NewCache(),
			}
			// Job doesn't exist → IgnoreNotFound → requeue, no error.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject a Build without jobRef at admission", func() {
			By("Creating a build with no jobRef")
			noJobRefBuild := &concoursev1alpha1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: "build-no-jobref", Namespace: "default"},
				Spec:       concoursev1alpha1.BuildSpec{},
			}
			err := k8sClient.Create(ctx, noJobRefBuild)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("jobRef is required"))
		})
	})

	Context("When build is in a terminal state", func() {
		ctx := context.Background()

		DescribeTable("should return no-op without status change",
			func(terminalStatus concoursev1alpha1.BuildPhase) {
				buildName := "build-terminal-" + string(terminalStatus)
				terminalBuild := &concoursev1alpha1.Build{
					ObjectMeta: metav1.ObjectMeta{Name: buildName, Namespace: "default"},
					Spec: concoursev1alpha1.BuildSpec{
						JobRef: &concoursev1alpha1.LocalObjectReference{Name: "some-job"},
					},
				}
				Expect(k8sClient.Create(ctx, terminalBuild)).To(Succeed())
				DeferCleanup(func() { _ = k8sClient.Delete(ctx, terminalBuild) })

				terminalBuild.Status.ConcourseStatus = terminalStatus
				Expect(k8sClient.Status().Update(ctx, terminalBuild)).To(Succeed())

				reconciler := &BuildReconciler{
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
				fetched := &concoursev1alpha1.Build{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: buildName, Namespace: "default"}, fetched)).To(Succeed())
				Expect(fetched.Status.ConcourseStatus).To(Equal(terminalStatus))
			},
			Entry("succeeded", concoursev1alpha1.BuildPhaseSucceeded),
			Entry("failed", concoursev1alpha1.BuildPhaseFailed),
			Entry("errored", concoursev1alpha1.BuildPhaseErrored),
			Entry("aborted", concoursev1alpha1.BuildPhaseAborted),
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

			notReadyPipeline := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "build-chain-pipeline", Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: "build-chain-team"},
					Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
				},
			}
			Expect(k8sClient.Create(ctx, notReadyPipeline)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, notReadyPipeline) })

			chainJob := &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "build-chain-job", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef: concoursev1alpha1.LocalObjectReference{Name: "build-chain-pipeline"},
					JobName:     "build",
				},
			}
			Expect(k8sClient.Create(ctx, chainJob)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, chainJob) })

			chainBuild := &concoursev1alpha1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: "build-chain-build", Namespace: "default"},
				Spec: concoursev1alpha1.BuildSpec{
					JobRef: &concoursev1alpha1.LocalObjectReference{Name: "build-chain-job"},
				},
			}
			Expect(k8sClient.Create(ctx, chainBuild)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, chainBuild) })

			reconciler := &BuildReconciler{
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
			fetched := &concoursev1alpha1.Build{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "build-chain-build", Namespace: "default"}, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("ChainNotReady"))
		})
	})

	Context("When Concourse API responds successfully", func() {
		ctx := context.Background()

		buildChain := func(cache *concourse.Cache, fake *fakeClient) (inst *concoursev1alpha1.Instance, team *concoursev1alpha1.Team, pipeline *concoursev1alpha1.Pipeline, job *concoursev1alpha1.Job) {
			inst = makeReadyInstanceWithFakeClient(ctx, "bc-inst", cache, fake)
			team = makeReadyTeam(ctx, "bc-team", inst.Name)
			pipeline = makeReadyPipeline(ctx, "bc-pipeline", team.Name)
			job = &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "bc-job", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef: concoursev1alpha1.LocalObjectReference{Name: pipeline.Name},
					JobName:     "deploy",
				},
			}
			return
		}

		It("should trigger a build and set BuildID/BuildName/APIURL on first reconcile", func() {
			cache := concourse.NewCache()
			fake := &fakeClient{
				team: &fakeTeam{
					name: "bc-team",
					createJobBuildFn: func(_ atc.PipelineRef, _ string) (atc.Build, error) {
						return atc.Build{ID: 101, Name: "1", APIURL: "/api/v1/builds/101"}, nil
					},
					pipelineFn: func(_ atc.PipelineRef) (atc.Pipeline, bool, error) { return atc.Pipeline{ID: 1}, true, nil },
				},
				buildFn:       func(_ string) (atc.Build, bool, error) { return atc.Build{ID: 101, Status: "pending"}, true, nil },
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst, team, pipeline, job := buildChain(cache, fake)
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, job)
				_ = k8sClient.Delete(ctx, pipeline)
				_ = k8sClient.Delete(ctx, team)
				_ = k8sClient.Delete(ctx, inst)
			})
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			buildCR := &concoursev1alpha1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: "build-trigger", Namespace: "default"},
				Spec:       concoursev1alpha1.BuildSpec{JobRef: &concoursev1alpha1.LocalObjectReference{Name: job.Name}},
			}
			Expect(k8sClient.Create(ctx, buildCR)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, buildCR) })

			reconciler := &BuildReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "build-trigger", Namespace: "default"}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0), "non-terminal build should requeue")

			By("Verifying BuildID, BuildName, APIURL in status")
			fetched := &concoursev1alpha1.Build{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.BuildID).NotTo(BeNil())
			Expect(*fetched.Status.BuildID).To(Equal(int32(101)))
			Expect(fetched.Status.BuildName).To(Equal("1"))
			Expect(fetched.Status.APIURL).To(Equal("/api/v1/builds/101"))
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should poll build status and set ConcourseStatus=succeeded when build finishes", func() {
			cache := concourse.NewCache()
			fake := &fakeClient{
				team: &fakeTeam{
					name: "bc2-team",
					createJobBuildFn: func(_ atc.PipelineRef, _ string) (atc.Build, error) {
						return atc.Build{ID: 202, Name: "2"}, nil
					},
				},
				buildFn: func(_ string) (atc.Build, bool, error) {
					return atc.Build{ID: 202, Status: "succeeded"}, true, nil
				},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst2 := makeReadyInstanceWithFakeClient(ctx, "bc2-inst", cache, fake)
			team2 := makeReadyTeam(ctx, "bc2-team", inst2.Name)
			pipeline2 := makeReadyPipeline(ctx, "bc2-pipeline", team2.Name)
			job2 := &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "bc2-job", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef: concoursev1alpha1.LocalObjectReference{Name: pipeline2.Name},
					JobName:     "deploy",
				},
			}
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, job2)
				_ = k8sClient.Delete(ctx, pipeline2)
				_ = k8sClient.Delete(ctx, team2)
				_ = k8sClient.Delete(ctx, inst2)
			})
			Expect(k8sClient.Create(ctx, job2)).To(Succeed())

			buildCR := &concoursev1alpha1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: "build-poll", Namespace: "default"},
				Spec:       concoursev1alpha1.BuildSpec{JobRef: &concoursev1alpha1.LocalObjectReference{Name: job2.Name}},
			}
			Expect(k8sClient.Create(ctx, buildCR)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, buildCR) })

			reconciler := &BuildReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "build-poll", Namespace: "default"}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			// build() returns succeeded, so no requeue
			Expect(result.RequeueAfter).To(BeZero(), "terminal build should not requeue")

			fetched := &concoursev1alpha1.Build{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.ConcourseStatus).To(Equal(concoursev1alpha1.BuildPhaseSucceeded))
		})

		It("should call AbortBuild when spec.canceled=true", func() {
			cache := concourse.NewCache()
			abortCalled := false
			fake := &fakeClient{
				team: &fakeTeam{
					name: "bc-team",
					createJobBuildFn: func(_ atc.PipelineRef, _ string) (atc.Build, error) {
						return atc.Build{ID: 303, Name: "3"}, nil
					},
				},
				buildFn:       func(_ string) (atc.Build, bool, error) { return atc.Build{ID: 303, Status: "started"}, true, nil },
				abortBuildFn:  func(_ string) error { abortCalled = true; return nil },
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst3 := makeReadyInstanceWithFakeClient(ctx, "bc3-inst", cache, fake)
			team3 := makeReadyTeam(ctx, "bc3-team", inst3.Name)
			pipeline3 := makeReadyPipeline(ctx, "bc3-pipeline", team3.Name)
			job3 := &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "bc3-job", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef: concoursev1alpha1.LocalObjectReference{Name: pipeline3.Name},
					JobName:     "deploy",
				},
			}
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, job3)
				_ = k8sClient.Delete(ctx, pipeline3)
				_ = k8sClient.Delete(ctx, team3)
				_ = k8sClient.Delete(ctx, inst3)
			})
			Expect(k8sClient.Create(ctx, job3)).To(Succeed())

			buildCR := &concoursev1alpha1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: "build-abort", Namespace: "default"},
				Spec: concoursev1alpha1.BuildSpec{
					JobRef:   &concoursev1alpha1.LocalObjectReference{Name: job3.Name},
					Canceled: true,
				},
			}
			Expect(k8sClient.Create(ctx, buildCR)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, buildCR) })

			reconciler := &BuildReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "build-abort", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(abortCalled).To(BeTrue(), "expected AbortBuild to be called")
		})

		It("should set Ready=False/TriggerFailed when CreateJobBuild returns error", func() {
			cache := concourse.NewCache()
			fake := &fakeClient{
				team: &fakeTeam{
					name: "bc-team",
					createJobBuildFn: func(_ atc.PipelineRef, _ string) (atc.Build, error) {
						return atc.Build{}, fmt.Errorf("job not found")
					},
				},
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst4 := makeReadyInstanceWithFakeClient(ctx, "bc4-inst", cache, fake)
			team4 := makeReadyTeam(ctx, "bc4-team", inst4.Name)
			pipeline4 := makeReadyPipeline(ctx, "bc4-pipeline", team4.Name)
			job4 := &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "bc4-job", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef: concoursev1alpha1.LocalObjectReference{Name: pipeline4.Name},
					JobName:     "deploy",
				},
			}
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, job4)
				_ = k8sClient.Delete(ctx, pipeline4)
				_ = k8sClient.Delete(ctx, team4)
				_ = k8sClient.Delete(ctx, inst4)
			})
			Expect(k8sClient.Create(ctx, job4)).To(Succeed())

			buildCR := &concoursev1alpha1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: "build-trigger-err", Namespace: "default"},
				Spec:       concoursev1alpha1.BuildSpec{JobRef: &concoursev1alpha1.LocalObjectReference{Name: job4.Name}},
			}
			Expect(k8sClient.Create(ctx, buildCR)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, buildCR) })

			reconciler := &BuildReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "build-trigger-err", Namespace: "default"}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			fetched := &concoursev1alpha1.Build{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("TriggerFailed"))
		})

		It("should call RerunJobBuild instead of CreateJobBuild when spec.rerunOf is set", func() {
			cache := concourse.NewCache()
			createCalled := false
			var rerunOfArg string
			fake := &fakeClient{
				team: &fakeTeam{
					name: "bc5-team",
					createJobBuildFn: func(_ atc.PipelineRef, _ string) (atc.Build, error) {
						createCalled = true
						return atc.Build{}, fmt.Errorf("CreateJobBuild should not be called when rerunOf is set")
					},
					rerunJobBuildFn: func(_ atc.PipelineRef, _ string, buildName string) (atc.Build, error) {
						rerunOfArg = buildName
						return atc.Build{ID: 404, Name: "5", APIURL: "/api/v1/builds/404"}, nil
					},
				},
				buildFn:       func(_ string) (atc.Build, bool, error) { return atc.Build{ID: 404, Status: "pending"}, true, nil },
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst5 := makeReadyInstanceWithFakeClient(ctx, "bc5-inst", cache, fake)
			team5 := makeReadyTeam(ctx, "bc5-team", inst5.Name)
			pipeline5 := makeReadyPipeline(ctx, "bc5-pipeline", team5.Name)
			job5 := &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "bc5-job", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef: concoursev1alpha1.LocalObjectReference{Name: pipeline5.Name},
					JobName:     "deploy",
				},
			}
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, job5)
				_ = k8sClient.Delete(ctx, pipeline5)
				_ = k8sClient.Delete(ctx, team5)
				_ = k8sClient.Delete(ctx, inst5)
			})
			Expect(k8sClient.Create(ctx, job5)).To(Succeed())

			buildCR := &concoursev1alpha1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: "build-rerun", Namespace: "default"},
				Spec: concoursev1alpha1.BuildSpec{
					JobRef:  &concoursev1alpha1.LocalObjectReference{Name: job5.Name},
					RerunOf: "42",
				},
			}
			Expect(k8sClient.Create(ctx, buildCR)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, buildCR) })

			reconciler := &BuildReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
			}
			nsn := types.NamespacedName{Name: "build-rerun", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())

			Expect(createCalled).To(BeFalse(), "CreateJobBuild should not be called when rerunOf is set")
			Expect(rerunOfArg).To(Equal("42"))

			fetched := &concoursev1alpha1.Build{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.BuildID).NotTo(BeNil())
			Expect(*fetched.Status.BuildID).To(Equal(int32(404)))
			Expect(fetched.Status.BuildName).To(Equal("5"))
			Expect(fetched.Status.RerunOf).To(Equal("42"))
		})

		It("should set a comment once and not re-set it when unchanged", func() {
			cache := concourse.NewCache()
			commentCalls := 0
			var lastComment string
			var lastBuildID int
			fake := &fakeClient{
				team: &fakeTeam{
					name: "bc6-team",
					createJobBuildFn: func(_ atc.PipelineRef, _ string) (atc.Build, error) {
						return atc.Build{ID: 505, Name: "6"}, nil
					},
				},
				buildFn:       func(_ string) (atc.Build, bool, error) { return atc.Build{ID: 505, Status: "pending"}, true, nil },
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst6 := makeReadyInstanceWithFakeClient(ctx, "bc6-inst", cache, fake)
			team6 := makeReadyTeam(ctx, "bc6-team", inst6.Name)
			pipeline6 := makeReadyPipeline(ctx, "bc6-pipeline", team6.Name)
			job6 := &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "bc6-job", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef: concoursev1alpha1.LocalObjectReference{Name: pipeline6.Name},
					JobName:     "deploy",
				},
			}
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, job6)
				_ = k8sClient.Delete(ctx, pipeline6)
				_ = k8sClient.Delete(ctx, team6)
				_ = k8sClient.Delete(ctx, inst6)
			})
			Expect(k8sClient.Create(ctx, job6)).To(Succeed())

			buildCR := &concoursev1alpha1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: "build-comment", Namespace: "default"},
				Spec: concoursev1alpha1.BuildSpec{
					JobRef:  &concoursev1alpha1.LocalObjectReference{Name: job6.Name},
					Comment: "first comment",
				},
			}
			Expect(k8sClient.Create(ctx, buildCR)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, buildCR) })

			reconciler := &BuildReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
				setBuildComment: func(_ context.Context, _ concourseapi.Client, buildID int, comment string) error {
					commentCalls++
					lastComment = comment
					lastBuildID = buildID
					return nil
				},
			}
			nsn := types.NamespacedName{Name: "build-comment", Namespace: "default"}

			// First reconcile: build gets triggered and observeBuild populates the
			// build ID; comment is set once against that build ID.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())

			Expect(commentCalls).To(Equal(1), "expected the comment to be set exactly once")
			Expect(lastComment).To(Equal("first comment"))
			Expect(lastBuildID).To(Equal(505))

			fetched := &concoursev1alpha1.Build{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.Comment).To(Equal("first comment"))

			// Second reconcile with the same spec.comment: the comment must not be
			// set again.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(commentCalls).To(Equal(1), "comment should not be re-set when unchanged")
		})

		It("should surface a comment failure as a Warning event and Ready=False without discarding the build", func() {
			cache := concourse.NewCache()
			fake := &fakeClient{
				team: &fakeTeam{
					name: "bc7-team",
					createJobBuildFn: func(_ atc.PipelineRef, _ string) (atc.Build, error) {
						return atc.Build{ID: 606, Name: "7"}, nil
					},
				},
				buildFn:       func(_ string) (atc.Build, bool, error) { return atc.Build{ID: 606, Status: "pending"}, true, nil },
				getInfoFn:     func() (atc.Info, error) { return atc.Info{}, nil },
				listWorkersFn: func() ([]atc.Worker, error) { return nil, nil },
			}
			inst7 := makeReadyInstanceWithFakeClient(ctx, "bc7-inst", cache, fake)
			team7 := makeReadyTeam(ctx, "bc7-team", inst7.Name)
			pipeline7 := makeReadyPipeline(ctx, "bc7-pipeline", team7.Name)
			job7 := &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "bc7-job", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef: concoursev1alpha1.LocalObjectReference{Name: pipeline7.Name},
					JobName:     "deploy",
				},
			}
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, job7)
				_ = k8sClient.Delete(ctx, pipeline7)
				_ = k8sClient.Delete(ctx, team7)
				_ = k8sClient.Delete(ctx, inst7)
			})
			Expect(k8sClient.Create(ctx, job7)).To(Succeed())

			buildCR := &concoursev1alpha1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: "build-comment-err", Namespace: "default"},
				Spec: concoursev1alpha1.BuildSpec{
					JobRef:  &concoursev1alpha1.LocalObjectReference{Name: job7.Name},
					Comment: "will fail",
				},
			}
			Expect(k8sClient.Create(ctx, buildCR)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, buildCR) })

			reconciler := &BuildReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Cache:  cache,
				setBuildComment: func(_ context.Context, _ concourseapi.Client, _ int, _ string) error {
					return fmt.Errorf("comment rejected")
				},
			}
			nsn := types.NamespacedName{Name: "build-comment-err", Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred(), "a comment failure should not fail the whole reconcile")

			fetched := &concoursev1alpha1.Build{}
			Expect(k8sClient.Get(ctx, nsn, fetched)).To(Succeed())

			// The build itself must still be tracked even though the comment failed.
			Expect(fetched.Status.BuildID).NotTo(BeNil())
			Expect(*fetched.Status.BuildID).To(Equal(int32(606)))
			Expect(fetched.Status.Comment).To(BeEmpty(), "comment must not be recorded on failure")

			cond := meta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("CommentFailed"))
		})
	})
})
