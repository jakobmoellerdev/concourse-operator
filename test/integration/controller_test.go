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

//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/concourse/concourse/atc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8smeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
	"github.com/jakobmoellerdev/concourse-operator/internal/controller"
)

// newEnvClient spins up an in-process envtest k8s API server and returns a
// client plus a teardown function. Call teardown in DeferCleanup/AfterEach.
func newEnvClient() (client.Client, func()) {
	Expect(concoursev1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: findEnvtestBinaries(),
	}
	cfg, err := env.Start()
	Expect(err).NotTo(HaveOccurred())

	k8s, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	return k8s, func() { _ = env.Stop() }
}

func findEnvtestBinaries() string {
	base := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			return filepath.Join(base, e.Name())
		}
	}
	return ""
}

// seedInstance creates an Instance CR, marks it Ready, and seeds the cache
// with the live concourseClient so controllers authenticate via the running
// Concourse rather than making a fresh HTTP auth attempt.
func seedInstance(ctx context.Context, k8s client.Client, cache *concourse.Cache, name string) *concoursev1alpha1.Instance {
	inst := &concoursev1alpha1.Instance{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       concoursev1alpha1.InstanceSpec{URL: concourseURL},
	}
	Expect(k8s.Create(ctx, inst)).To(Succeed())
	inst.Status.Conditions = []metav1.Condition{{
		Type:               concoursev1alpha1.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Ready",
		LastTransitionTime: metav1.Now(),
	}}
	Expect(k8s.Status().Update(ctx, inst)).To(Succeed())

	// Re-fetch to capture the ResourceVersion after the status update.
	latest := &concoursev1alpha1.Instance{}
	Expect(k8s.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, latest)).To(Succeed())
	cache.Set(latest, concourseClient)
	return latest
}

// makeReadyTeamCR creates a Team CR with Ready=True, referencing the given instance.
func makeReadyTeamCR(ctx context.Context, k8s client.Client, name, instanceName string) *concoursev1alpha1.Team {
	team := &concoursev1alpha1.Team{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: concoursev1alpha1.TeamSpec{
			InstanceRef: concoursev1alpha1.LocalObjectReference{Name: instanceName},
		},
	}
	Expect(k8s.Create(ctx, team)).To(Succeed())
	team.Status.Conditions = []metav1.Condition{{
		Type:               concoursev1alpha1.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Ready",
		LastTransitionTime: metav1.Now(),
	}}
	Expect(k8s.Status().Update(ctx, team)).To(Succeed())
	return team
}

// makeReadyPipelineCR creates a Pipeline CR with Ready=True.
func makeReadyPipelineCR(ctx context.Context, k8s client.Client, name, teamName string) *concoursev1alpha1.Pipeline {
	pl := &concoursev1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: concoursev1alpha1.PipelineSpec{
			TeamRef: concoursev1alpha1.LocalObjectReference{Name: teamName},
			Config:  concoursev1alpha1.PipelineConfig{Inline: minimalJobPipeline},
		},
	}
	Expect(k8s.Create(ctx, pl)).To(Succeed())
	pl.Status.Conditions = []metav1.Condition{{
		Type:               concoursev1alpha1.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Ready",
		LastTransitionTime: metav1.Now(),
	}}
	Expect(k8s.Status().Update(ctx, pl)).To(Succeed())
	return pl
}

// reconcileTeam runs TeamReconciler twice: once to add the finalizer, once to
// call CreateOrUpdate against Concourse.
func reconcileTeam(ctx context.Context, k8s client.Client, cache *concourse.Cache, teamName string) {
	r := &controller.TeamReconciler{Client: k8s, Scheme: k8s.Scheme(), Cache: cache}
	nsn := types.NamespacedName{Name: teamName, Namespace: "default"}
	_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
	Expect(err).NotTo(HaveOccurred())
	_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
	Expect(err).NotTo(HaveOccurred())
}

// reconcilePipeline runs PipelineReconciler twice: finalizer then apply.
func reconcilePipeline(ctx context.Context, k8s client.Client, cache *concourse.Cache, plName string) {
	r := &controller.PipelineReconciler{Client: k8s, Scheme: k8s.Scheme(), Cache: cache}
	nsn := types.NamespacedName{Name: plName, Namespace: "default"}
	_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
	Expect(err).NotTo(HaveOccurred())
	_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
	Expect(err).NotTo(HaveOccurred())
}

const minimalJobPipeline = `jobs:
- name: hello
  plan:
  - task: say-hello
    config:
      platform: linux
      image_resource:
        type: registry-image
        source:
          repository: alpine
      run:
        path: echo
        args: ["Hello from operator test"]
`

var _ = Describe("Controller Integration", func() {
	var (
		ctx  context.Context
		k8s  client.Client
		stop func()
	)

	BeforeEach(func() {
		ctx = context.Background()
		k8s, stop = newEnvClient()
	})

	AfterEach(func() { stop() })

	// -----------------------------------------------------------------------
	Describe("InstanceReconciler", func() {
		const instanceFinalizer = "concourse-ci.org/instance-finalizer"

		It("connects to real Concourse and sets Ready=True, Version, WorkerCount", func() {
			cache := concourse.NewCache()
			nsn := types.NamespacedName{Name: "ci-inst", Namespace: "default"}

			inst := &concoursev1alpha1.Instance{
				ObjectMeta: metav1.ObjectMeta{Name: "ci-inst", Namespace: "default"},
				Spec:       concoursev1alpha1.InstanceSpec{URL: concourseURL},
			}
			Expect(k8s.Create(ctx, inst)).To(Succeed())
			DeferCleanup(func() {
				latest := &concoursev1alpha1.Instance{}
				if err := k8s.Get(ctx, nsn, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, instanceFinalizer)
					_ = k8s.Update(ctx, latest)
					_ = k8s.Delete(ctx, latest)
				}
			})

			r := &controller.InstanceReconciler{Client: k8s, Scheme: k8s.Scheme(), Cache: cache}

			By("First reconcile adds finalizer (no cached client yet → builds one via HTTP auth)")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())

			By("Seed cache with live client at the post-finalizer ResourceVersion")
			afterFinalizer := &concoursev1alpha1.Instance{}
			Expect(k8s.Get(ctx, nsn, afterFinalizer)).To(Succeed())
			cache.Set(afterFinalizer, concourseClient)

			By("Second reconcile calls GetInfo + ListWorkers against real Concourse")
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nsn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			fetched := &concoursev1alpha1.Instance{}
			Expect(k8s.Get(ctx, nsn, fetched)).To(Succeed())
			Expect(fetched.Status.Version).NotTo(BeEmpty())
			Expect(fetched.Status.WorkerCount).To(BeNumerically(">=", 1))
			cond := k8smeta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	// -----------------------------------------------------------------------
	Describe("TeamReconciler", func() {
		const teamFinalizer = "concourse-ci.org/team-finalizer"

		It("creates the team in Concourse and sets status.TeamID", func() {
			const (
				instName = "ci-team-inst"
				teamName = "ctrl-integ-team-create"
			)
			DeferCleanup(func() { _ = concourseClient.Team(teamName).DestroyTeam(teamName) })

			cache := concourse.NewCache()
			inst := seedInstance(ctx, k8s, cache, instName)
			DeferCleanup(func() { _ = k8s.Delete(ctx, inst) })

			teamCR := &concoursev1alpha1.Team{
				ObjectMeta: metav1.ObjectMeta{Name: teamName, Namespace: "default"},
				Spec: concoursev1alpha1.TeamSpec{
					InstanceRef: concoursev1alpha1.LocalObjectReference{Name: instName},
				},
			}
			Expect(k8s.Create(ctx, teamCR)).To(Succeed())
			teamNSN := types.NamespacedName{Name: teamName, Namespace: "default"}
			DeferCleanup(func() {
				latest := &concoursev1alpha1.Team{}
				if err := k8s.Get(ctx, teamNSN, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, teamFinalizer)
					_ = k8s.Update(ctx, latest)
					_ = k8s.Delete(ctx, latest)
				}
			})

			r := &controller.TeamReconciler{Client: k8s, Scheme: k8s.Scheme(), Cache: cache}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: teamNSN})
			Expect(err).NotTo(HaveOccurred())
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: teamNSN})
			Expect(err).NotTo(HaveOccurred())

			fetched := &concoursev1alpha1.Team{}
			Expect(k8s.Get(ctx, teamNSN, fetched)).To(Succeed())
			Expect(fetched.Status.TeamID).To(BeNumerically(">", 0))
			cond := k8smeta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))

			teams, err := concourseClient.ListTeams()
			Expect(err).NotTo(HaveOccurred())
			names := make([]string, len(teams))
			for i, t := range teams {
				names[i] = t.Name
			}
			Expect(names).To(ContainElement(teamName))
		})

		It("removes the team from Concourse when the CR is deleted", func() {
			const (
				instName = "ci-team-del-inst"
				teamName = "ctrl-integ-team-del"
			)
			DeferCleanup(func() { _ = concourseClient.Team(teamName).DestroyTeam(teamName) })

			cache := concourse.NewCache()
			inst := seedInstance(ctx, k8s, cache, instName)
			DeferCleanup(func() { _ = k8s.Delete(ctx, inst) })

			teamCR := &concoursev1alpha1.Team{
				ObjectMeta: metav1.ObjectMeta{Name: teamName, Namespace: "default"},
				Spec: concoursev1alpha1.TeamSpec{
					InstanceRef: concoursev1alpha1.LocalObjectReference{Name: instName},
				},
			}
			Expect(k8s.Create(ctx, teamCR)).To(Succeed())
			teamNSN := types.NamespacedName{Name: teamName, Namespace: "default"}

			r := &controller.TeamReconciler{Client: k8s, Scheme: k8s.Scheme(), Cache: cache}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: teamNSN})
			Expect(err).NotTo(HaveOccurred())
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: teamNSN})
			Expect(err).NotTo(HaveOccurred())

			fetched := &concoursev1alpha1.Team{}
			Expect(k8s.Get(ctx, teamNSN, fetched)).To(Succeed())
			Expect(fetched.Status.TeamID).To(BeNumerically(">", 0))

			By("Deleting the CR triggers finalization → DestroyTeam in Concourse")
			Expect(k8s.Delete(ctx, fetched)).To(Succeed())
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: teamNSN})
			Expect(err).NotTo(HaveOccurred())

			teams, err := concourseClient.ListTeams()
			Expect(err).NotTo(HaveOccurred())
			for _, t := range teams {
				Expect(t.Name).NotTo(Equal(teamName))
			}
		})
	})

	// -----------------------------------------------------------------------
	Describe("PipelineReconciler", func() {
		const pipelineFinalizer = "concourse-ci.org/pipeline-finalizer"

		var (
			cache    *concourse.Cache
			inst     *concoursev1alpha1.Instance
			instName string
			teamName string
		)

		BeforeEach(func() {
			cache = concourse.NewCache()
			// Use Ginkgo node index to avoid name collisions when running
			// multiple parallel processes.
			instName = fmt.Sprintf("ci-pl-inst-%d", GinkgoParallelProcess())
			teamName = fmt.Sprintf("ctrl-integ-pl-team-%d", GinkgoParallelProcess())

			inst = seedInstance(ctx, k8s, cache, instName)
			_ = makeReadyTeamCR(ctx, k8s, teamName, instName)
			reconcileTeam(ctx, k8s, cache, teamName)

			DeferCleanup(func() {
				_ = concourseClient.Team(teamName).DestroyTeam(teamName)
				_ = k8s.Delete(ctx, inst)
			})
		})

		It("applies inline config and sets PipelineID and ConfigHash", func() {
			const plName = "ctrl-integ-pl-apply"
			pl := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: plName, Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: teamName},
					Config:  concoursev1alpha1.PipelineConfig{Inline: minimalJobPipeline},
				},
			}
			Expect(k8s.Create(ctx, pl)).To(Succeed())
			plNSN := types.NamespacedName{Name: plName, Namespace: "default"}
			DeferCleanup(func() {
				latest := &concoursev1alpha1.Pipeline{}
				if err := k8s.Get(ctx, plNSN, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, pipelineFinalizer)
					_ = k8s.Update(ctx, latest)
					_ = k8s.Delete(ctx, latest)
				}
				_, _ = concourseClient.Team(teamName).DeletePipeline(atc.PipelineRef{Name: plName})
			})

			reconcilePipeline(ctx, k8s, cache, plName)

			fetched := &concoursev1alpha1.Pipeline{}
			Expect(k8s.Get(ctx, plNSN, fetched)).To(Succeed())
			Expect(fetched.Status.PipelineID).To(BeNumerically(">", 0))
			Expect(fetched.Status.ConfigHash).NotTo(BeEmpty())
			cond := k8smeta.FindStatusCondition(fetched.Status.Conditions, concoursev1alpha1.ConditionReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))

			// Verify the pipeline actually exists in Concourse.
			_, found, err := concourseClient.Team(teamName).Pipeline(atc.PipelineRef{Name: plName})
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
		})

		It("pauses the pipeline in Concourse when spec.paused=true", func() {
			const plName = "ctrl-integ-pl-pause"
			pl := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: plName, Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: teamName},
					Config:  concoursev1alpha1.PipelineConfig{Inline: minimalJobPipeline},
					Paused:  true,
				},
			}
			Expect(k8s.Create(ctx, pl)).To(Succeed())
			plNSN := types.NamespacedName{Name: plName, Namespace: "default"}
			DeferCleanup(func() {
				latest := &concoursev1alpha1.Pipeline{}
				if err := k8s.Get(ctx, plNSN, latest); err == nil {
					controllerutil.RemoveFinalizer(latest, pipelineFinalizer)
					_ = k8s.Update(ctx, latest)
					_ = k8s.Delete(ctx, latest)
				}
				_, _ = concourseClient.Team(teamName).DeletePipeline(atc.PipelineRef{Name: plName})
			})

			reconcilePipeline(ctx, k8s, cache, plName)

			fetched := &concoursev1alpha1.Pipeline{}
			Expect(k8s.Get(ctx, plNSN, fetched)).To(Succeed())
			Expect(fetched.Status.Paused).To(BeTrue())

			p, found, err := concourseClient.Team(teamName).Pipeline(atc.PipelineRef{Name: plName})
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(p.Paused).To(BeTrue())
		})

		It("deletes the pipeline from Concourse when the CR is deleted", func() {
			const plName = "ctrl-integ-pl-del"
			pl := &concoursev1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{Name: plName, Namespace: "default"},
				Spec: concoursev1alpha1.PipelineSpec{
					TeamRef: concoursev1alpha1.LocalObjectReference{Name: teamName},
					Config:  concoursev1alpha1.PipelineConfig{Inline: minimalJobPipeline},
				},
			}
			Expect(k8s.Create(ctx, pl)).To(Succeed())
			plNSN := types.NamespacedName{Name: plName, Namespace: "default"}
			DeferCleanup(func() {
				_, _ = concourseClient.Team(teamName).DeletePipeline(atc.PipelineRef{Name: plName})
			})

			reconcilePipeline(ctx, k8s, cache, plName)

			_, found, err := concourseClient.Team(teamName).Pipeline(atc.PipelineRef{Name: plName})
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())

			By("Deleting the CR triggers finalization → DeletePipeline in Concourse")
			fetched := &concoursev1alpha1.Pipeline{}
			Expect(k8s.Get(ctx, plNSN, fetched)).To(Succeed())
			Expect(k8s.Delete(ctx, fetched)).To(Succeed())

			r := &controller.PipelineReconciler{Client: k8s, Scheme: k8s.Scheme(), Cache: cache}
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: plNSN})
			Expect(err).NotTo(HaveOccurred())

			_, found, err = concourseClient.Team(teamName).Pipeline(atc.PipelineRef{Name: plName})
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeFalse(), "pipeline should be gone from Concourse after CR deletion")
		})
	})

	// -----------------------------------------------------------------------
	Describe("BuildReconciler", func() {
		It("triggers a job build in Concourse and sets BuildID in status", func() {
			const (
				instName = "ci-build-inst"
				teamName = "ctrl-integ-build-team"
				plName   = "ctrl-integ-build-pl"
				jobName  = "hello"
			)

			cache := concourse.NewCache()
			inst := seedInstance(ctx, k8s, cache, instName)
			DeferCleanup(func() { _ = k8s.Delete(ctx, inst) })

			_ = makeReadyTeamCR(ctx, k8s, teamName, instName)
			reconcileTeam(ctx, k8s, cache, teamName)
			DeferCleanup(func() { _ = concourseClient.Team(teamName).DestroyTeam(teamName) })

			_ = makeReadyPipelineCR(ctx, k8s, plName, teamName)
			reconcilePipeline(ctx, k8s, cache, plName)
			DeferCleanup(func() {
				_, _ = concourseClient.Team(teamName).DeletePipeline(atc.PipelineRef{Name: plName})
			})

			// Unpause so builds can be triggered.
			_, err := concourseClient.Team(teamName).UnpausePipeline(atc.PipelineRef{Name: plName})
			Expect(err).NotTo(HaveOccurred())

			// Create a Job CR (Ready=True so the Build resolver can follow the ref).
			jobCR := &concoursev1alpha1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "ci-build-job", Namespace: "default"},
				Spec: concoursev1alpha1.JobSpec{
					PipelineRef: concoursev1alpha1.LocalObjectReference{Name: plName},
					JobName:     jobName,
				},
			}
			Expect(k8s.Create(ctx, jobCR)).To(Succeed())
			jobCR.Status.Conditions = []metav1.Condition{{
				Type:               concoursev1alpha1.ConditionReady,
				Status:             metav1.ConditionTrue,
				Reason:             "Ready",
				LastTransitionTime: metav1.Now(),
			}}
			Expect(k8s.Status().Update(ctx, jobCR)).To(Succeed())
			DeferCleanup(func() { _ = k8s.Delete(ctx, jobCR) })

			buildCR := &concoursev1alpha1.Build{
				ObjectMeta: metav1.ObjectMeta{Name: "ci-build-cr", Namespace: "default"},
				Spec: concoursev1alpha1.BuildSpec{
					JobRef: &concoursev1alpha1.LocalObjectReference{Name: "ci-build-job"},
				},
			}
			Expect(k8s.Create(ctx, buildCR)).To(Succeed())
			buildNSN := types.NamespacedName{Name: "ci-build-cr", Namespace: "default"}
			DeferCleanup(func() { _ = k8s.Delete(ctx, buildCR) })

			By("Reconciling triggers CreateJobBuild in Concourse")
			r := &controller.BuildReconciler{Client: k8s, Scheme: k8s.Scheme(), Cache: cache}
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: buildNSN})
			Expect(err).NotTo(HaveOccurred())

			fetched := &concoursev1alpha1.Build{}
			Expect(k8s.Get(ctx, buildNSN, fetched)).To(Succeed())
			Expect(fetched.Status.BuildID).To(BeNumerically(">", 0), "BuildID must be set after triggering")
			Expect(fetched.Status.BuildName).NotTo(BeEmpty())
			GinkgoWriter.Printf("Concourse build triggered: ID=%d name=%s status=%s\n",
				fetched.Status.BuildID, fetched.Status.BuildName, fetched.Status.ConcourseStatus)
		})
	})
})
