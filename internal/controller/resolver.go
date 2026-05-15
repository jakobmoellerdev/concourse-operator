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

	goconcourse "github.com/concourse/concourse/go-concourse/concourse"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

// resolveInstanceForTeam returns the ConcourseInstance and Concourse client for a team.
func resolveInstanceForTeam(ctx context.Context, k8sClient client.Client, cache *concourse.Cache, team *concoursev1alpha1.ConcourseTeam) (*concoursev1alpha1.ConcourseInstance, goconcourse.Client, error) {
	instance := &concoursev1alpha1.ConcourseInstance{}
	key := client.ObjectKey{Namespace: team.Namespace, Name: team.Spec.InstanceRef.Name}
	if err := k8sClient.Get(ctx, key, instance); err != nil {
		return nil, nil, fmt.Errorf("get instance %s: %w", team.Spec.InstanceRef.Name, err)
	}
	if !meta.IsStatusConditionTrue(instance.Status.Conditions, concoursev1alpha1.ConditionReady) {
		return nil, nil, fmt.Errorf("instance %s is not ready", instance.Name)
	}
	cl, err := cache.GetOrBuild(ctx, k8sClient, instance)
	if err != nil {
		return nil, nil, fmt.Errorf("get client for instance %s: %w", instance.Name, err)
	}
	return instance, cl, nil
}

// resolveClientForPipeline returns the Concourse client and team name for a pipeline.
func resolveClientForPipeline(ctx context.Context, k8sClient client.Client, cache *concourse.Cache, pipeline *concoursev1alpha1.ConcoursePipeline) (goconcourse.Client, string, error) {
	team := &concoursev1alpha1.ConcourseTeam{}
	key := client.ObjectKey{Namespace: pipeline.Namespace, Name: pipeline.Spec.TeamRef.Name}
	if err := k8sClient.Get(ctx, key, team); err != nil {
		return nil, "", fmt.Errorf("get team %s: %w", pipeline.Spec.TeamRef.Name, err)
	}
	if !meta.IsStatusConditionTrue(team.Status.Conditions, concoursev1alpha1.ConditionReady) {
		return nil, "", fmt.Errorf("team %s is not ready", team.Name)
	}
	_, cl, err := resolveInstanceForTeam(ctx, k8sClient, cache, team)
	if err != nil {
		return nil, "", err
	}
	teamName := team.Spec.TeamName
	if teamName == "" {
		teamName = team.Name
	}
	return cl, teamName, nil
}

// resolveClientForJob returns the Concourse client, team name, and pipeline name for a job.
func resolveClientForJob(ctx context.Context, k8sClient client.Client, cache *concourse.Cache, job *concoursev1alpha1.ConcourseJob) (goconcourse.Client, string, string, error) {
	pipeline := &concoursev1alpha1.ConcoursePipeline{}
	key := client.ObjectKey{Namespace: job.Namespace, Name: job.Spec.PipelineRef.Name}
	if err := k8sClient.Get(ctx, key, pipeline); err != nil {
		return nil, "", "", fmt.Errorf("get pipeline %s: %w", job.Spec.PipelineRef.Name, err)
	}
	if !meta.IsStatusConditionTrue(pipeline.Status.Conditions, concoursev1alpha1.ConditionReady) {
		return nil, "", "", fmt.Errorf("pipeline %s is not ready", pipeline.Name)
	}
	cl, teamName, err := resolveClientForPipeline(ctx, k8sClient, cache, pipeline)
	if err != nil {
		return nil, "", "", err
	}
	pipelineName := pipeline.Spec.PipelineName
	if pipelineName == "" {
		pipelineName = pipeline.Name
	}
	return cl, teamName, pipelineName, nil
}

// setCondition updates or appends a condition in the slice (defined in conditions.go).
// Kept here as documentation — actual implementation is in conditions.go.

// isReady returns true if the Ready condition is true.
func isReady(conditions []metav1.Condition) bool {
	return meta.IsStatusConditionTrue(conditions, concoursev1alpha1.ConditionReady)
}
