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

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
)

// makeReadyInstance creates a ConcourseInstance with Ready=True status.
func makeReadyInstance(ctx context.Context, name string) *concoursev1alpha1.ConcourseInstance {
	inst := &concoursev1alpha1.ConcourseInstance{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       concoursev1alpha1.ConcourseInstanceSpec{URL: "https://ci.example.com"},
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

// makeReadyTeam creates a ConcourseTeam with Ready=True status.
func makeReadyTeam(ctx context.Context, name, instanceName string) *concoursev1alpha1.ConcourseTeam {
	team := &concoursev1alpha1.ConcourseTeam{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: concoursev1alpha1.ConcourseTeamSpec{
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

// makeReadyPipeline creates a ConcoursePipeline with Ready=True status.
func makeReadyPipeline(ctx context.Context, name, teamName string) *concoursev1alpha1.ConcoursePipeline {
	pipeline := &concoursev1alpha1.ConcoursePipeline{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: concoursev1alpha1.ConcoursePipelineSpec{
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

var _ = Describe("isTerminal", func() {
	DescribeTable("build status terminal detection",
		func(status concoursev1alpha1.BuildStatus, expected bool) {
			Expect(isTerminal(status)).To(Equal(expected))
		},
		Entry("empty is not terminal", concoursev1alpha1.BuildStatus(""), false),
		Entry("pending is not terminal", concoursev1alpha1.BuildStatusPending, false),
		Entry("started is not terminal", concoursev1alpha1.BuildStatusStarted, false),
		Entry("succeeded is terminal", concoursev1alpha1.BuildStatusSucceeded, true),
		Entry("failed is terminal", concoursev1alpha1.BuildStatusFailed, true),
		Entry("errored is terminal", concoursev1alpha1.BuildStatusErrored, true),
		Entry("aborted is terminal", concoursev1alpha1.BuildStatusAborted, true),
	)
})

var _ = Describe("shouldCheck", func() {
	It("returns false when CheckInterval is nil", func() {
		r := &concoursev1alpha1.ConcourseResource{}
		Expect(shouldCheck(r)).To(BeFalse())
	})

	It("returns true when CheckInterval is set and never checked", func() {
		d := metav1.Duration{Duration: 5 * time.Minute}
		r := &concoursev1alpha1.ConcourseResource{
			Spec: concoursev1alpha1.ConcourseResourceSpec{CheckInterval: &d},
		}
		Expect(shouldCheck(r)).To(BeTrue())
	})

	It("returns false when last checked recently", func() {
		d := metav1.Duration{Duration: 5 * time.Minute}
		now := metav1.Now()
		r := &concoursev1alpha1.ConcourseResource{
			Spec:   concoursev1alpha1.ConcourseResourceSpec{CheckInterval: &d},
			Status: concoursev1alpha1.ConcourseResourceStatus{LastChecked: &now},
		}
		Expect(shouldCheck(r)).To(BeFalse())
	})

	It("returns true when check interval has elapsed", func() {
		d := metav1.Duration{Duration: time.Millisecond}
		past := metav1.NewTime(time.Now().Add(-time.Hour))
		r := &concoursev1alpha1.ConcourseResource{
			Spec:   concoursev1alpha1.ConcourseResourceSpec{CheckInterval: &d},
			Status: concoursev1alpha1.ConcourseResourceStatus{LastChecked: &past},
		}
		Expect(shouldCheck(r)).To(BeTrue())
	})
})
