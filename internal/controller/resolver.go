/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/License-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on "AS IS" BASIS,
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

func refKey(ns string, ref concoursev1alpha1.LocalObjectReference) client.ObjectKey {
	return client.ObjectKey{Namespace: ref.ResolveNamespace(ns), Name: ref.Name}
}

func namespaceAllowed(instance *concoursev1alpha1.Instance, ns string) error {
	if ns == instance.Namespace {
		return nil
	}
	for _, allowed := range instance.Spec.AllowedNamespaces {
		if allowed == ns || allowed == "*" {
			return nil
		}
	}
	return fmt.Errorf("namespace %q is not allowed to reference instance %s/%s", ns, instance.Namespace, instance.Name)
}

// resolveInstanceForTeam returns the Concourse client for a team.
func resolveInstanceForTeam(ctx context.Context, k8sClient client.Client, cache *concourse.Cache, team *concoursev1alpha1.Team) (goconcourse.Client, error) {
	instance := &concoursev1alpha1.Instance{}
	if err := k8sClient.Get(ctx, refKey(team.Namespace, team.Spec.InstanceRef), instance); err != nil {
		return nil, fmt.Errorf("get instance %s: %w", team.Spec.InstanceRef.Name, err)
	}
	if err := namespaceAllowed(instance, team.Namespace); err != nil {
		return nil, err
	}
	if !meta.IsStatusConditionTrue(instance.Status.Conditions, concoursev1alpha1.ConditionReady) {
		return nil, fmt.Errorf("instance %s is not ready", instance.Name)
	}
	cl, err := cache.GetOrBuild(ctx, k8sClient, instance)
	if err != nil {
		return nil, fmt.Errorf("get client for instance %s: %w", instance.Name, err)
	}
	return cl, nil
}

// resolveClientForPipeline returns the Concourse client and team name for a pipeline.
func resolveClientForPipeline(ctx context.Context, k8sClient client.Client, cache *concourse.Cache, pipeline *concoursev1alpha1.Pipeline) (goconcourse.Client, string, error) {
	team := &concoursev1alpha1.Team{}
	if err := k8sClient.Get(ctx, refKey(pipeline.Namespace, pipeline.Spec.TeamRef), team); err != nil {
		return nil, "", fmt.Errorf("get team %s: %w", pipeline.Spec.TeamRef.Name, err)
	}
	if !meta.IsStatusConditionTrue(team.Status.Conditions, concoursev1alpha1.ConditionReady) {
		return nil, "", fmt.Errorf("team %s is not ready", team.Name)
	}
	cl, err := resolveInstanceForTeam(ctx, k8sClient, cache, team)
	if err != nil {
		return nil, "", err
	}
	return cl, concoursev1alpha1.ResolvedTeamName(team), nil
}

// resolveClientForJob returns the Concourse client, team name, and pipeline name for a job.
func resolveClientForJob(ctx context.Context, k8sClient client.Client, cache *concourse.Cache, job *concoursev1alpha1.Job) (goconcourse.Client, string, string, error) {
	pipeline := &concoursev1alpha1.Pipeline{}
	if err := k8sClient.Get(ctx, refKey(job.Namespace, job.Spec.PipelineRef), pipeline); err != nil {
		return nil, "", "", fmt.Errorf("get pipeline %s: %w", job.Spec.PipelineRef.Name, err)
	}
	if !meta.IsStatusConditionTrue(pipeline.Status.Conditions, concoursev1alpha1.ConditionReady) {
		return nil, "", "", fmt.Errorf("pipeline %s is not ready", pipeline.Name)
	}
	cl, teamName, err := resolveClientForPipeline(ctx, k8sClient, cache, pipeline)
	if err != nil {
		return nil, "", "", err
	}
	return cl, teamName, concoursev1alpha1.ResolvedPipelineName(pipeline), nil
}

// isReady returns true if the Ready condition is true.
func isReady(conditions []metav1.Condition) bool {
	return meta.IsStatusConditionTrue(conditions, concoursev1alpha1.ConditionReady)
}
