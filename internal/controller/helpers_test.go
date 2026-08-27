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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

func testInstanceAuth() concoursev1alpha1.InstanceAuth {
	return concoursev1alpha1.InstanceAuth{
		Password: &concoursev1alpha1.PasswordGrant{
			Username:    "test",
			PasswordRef: concoursev1alpha1.SecretKeySelector{Name: "concourse-local-credentials", Key: "password"},
		},
	}
}

func testInstanceSpec() concoursev1alpha1.InstanceSpec {
	return concoursev1alpha1.InstanceSpec{
		URL:  "https://ci.example.com",
		Auth: testInstanceAuth(),
	}
}

// makeReadyInstance creates a Instance with Ready=True status.
func makeReadyInstance(ctx context.Context, name string) *concoursev1alpha1.Instance {
	inst := &concoursev1alpha1.Instance{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       testInstanceSpec(),
	}
	Expect(k8sClient.Create(ctx, inst)).To(Succeed())
	inst.Status.Conditions = []metav1.Condition{{
		Type:               concoursev1alpha1.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Ready",
		LastTransitionTime: metav1.Now(),
	}}
	Expect(k8sClient.Status().Update(ctx, inst)).To(Succeed())
	return inst
}

// makeReadyTeam creates a Team with Ready=True status.
func makeReadyTeam(ctx context.Context, name, instanceName string) *concoursev1alpha1.Team {
	team := &concoursev1alpha1.Team{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: concoursev1alpha1.TeamSpec{
			InstanceRef: concoursev1alpha1.LocalObjectReference{Name: instanceName},
		},
	}
	Expect(k8sClient.Create(ctx, team)).To(Succeed())
	team.Status.Conditions = []metav1.Condition{{
		Type:               concoursev1alpha1.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Ready",
		LastTransitionTime: metav1.Now(),
	}}
	Expect(k8sClient.Status().Update(ctx, team)).To(Succeed())
	return team
}

// makeReadyPipeline creates a Pipeline with Ready=True status.
func makeReadyPipeline(ctx context.Context, name, teamName string) *concoursev1alpha1.Pipeline {
	pipeline := &concoursev1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: concoursev1alpha1.PipelineSpec{
			TeamRef: concoursev1alpha1.LocalObjectReference{Name: teamName},
			Config:  concoursev1alpha1.PipelineConfig{Inline: "jobs: []"},
		},
	}
	Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
	pipeline.Status.Conditions = []metav1.Condition{{
		Type:               concoursev1alpha1.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Ready",
		LastTransitionTime: metav1.Now(),
	}}
	Expect(k8sClient.Status().Update(ctx, pipeline)).To(Succeed())
	return pipeline
}

// makeReadyInstanceWithFakeClient creates a ready ConcourseInstance and pre-populates
// the given cache with the provided fakeClient so reconcilers skip HTTP auth.
// It re-fetches the instance after creation to ensure the ResourceVersion is current.
func makeReadyInstanceWithFakeClient(ctx context.Context, name string, c *concourse.Cache, cl *fakeClient) *concoursev1alpha1.Instance {
	makeReadyInstance(ctx, name)
	// Re-fetch to get the latest ResourceVersion (status update above may have bumped it).
	latest := &concoursev1alpha1.Instance{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, latest)).To(Succeed())
	c.Set(latest, cl)
	return latest
}

var _ = Describe("isTerminal", func() {
	DescribeTable("build status terminal detection",
		func(status concoursev1alpha1.BuildPhase, expected bool) {
			Expect(isTerminal(status)).To(Equal(expected))
		},
		Entry("empty is not terminal", concoursev1alpha1.BuildPhase(""), false),
		Entry("pending is not terminal", concoursev1alpha1.BuildPhasePending, false),
		Entry("started is not terminal", concoursev1alpha1.BuildPhaseStarted, false),
		Entry("succeeded is terminal", concoursev1alpha1.BuildPhaseSucceeded, true),
		Entry("failed is terminal", concoursev1alpha1.BuildPhaseFailed, true),
		Entry("errored is terminal", concoursev1alpha1.BuildPhaseErrored, true),
		Entry("aborted is terminal", concoursev1alpha1.BuildPhaseAborted, true),
	)
})

var _ = Describe("shouldCheck", func() {
	It("returns false when CheckInterval is nil", func() {
		r := &concoursev1alpha1.Resource{}
		Expect(shouldCheck(r)).To(BeFalse())
	})

	It("returns true when CheckInterval is set and never checked", func() {
		d := metav1.Duration{Duration: 5 * time.Minute}
		r := &concoursev1alpha1.Resource{
			Spec: concoursev1alpha1.ResourceSpec{CheckInterval: &d},
		}
		Expect(shouldCheck(r)).To(BeTrue())
	})

	It("returns false when last checked recently", func() {
		d := metav1.Duration{Duration: 5 * time.Minute}
		now := metav1.Now()
		r := &concoursev1alpha1.Resource{
			Spec:   concoursev1alpha1.ResourceSpec{CheckInterval: &d},
			Status: concoursev1alpha1.ResourceStatus{LastChecked: &now},
		}
		Expect(shouldCheck(r)).To(BeFalse())
	})

	It("returns true when check interval has elapsed", func() {
		d := metav1.Duration{Duration: time.Millisecond}
		past := metav1.NewTime(time.Now().Add(-time.Hour))
		r := &concoursev1alpha1.Resource{
			Spec:   concoursev1alpha1.ResourceSpec{CheckInterval: &d},
			Status: concoursev1alpha1.ResourceStatus{LastChecked: &past},
		}
		Expect(shouldCheck(r)).To(BeTrue())
	})
})
