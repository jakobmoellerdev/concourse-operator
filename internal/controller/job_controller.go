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
	"time"

	"github.com/concourse/concourse/atc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

// JobReconciler reconciles a Job object.
type JobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Cache  *concourse.Cache
}

// +kubebuilder:rbac:groups=concourse-ci.org,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=concourse-ci.org,resources=jobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=jobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=concourse-ci.org,resources=pipelines,verbs=get;list;watch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=teams,verbs=get;list;watch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=builds,verbs=get;list;watch;create;update;patch

func (r *JobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	job := &concoursev1alpha1.Job{}
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !job.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	cl, teamName, pipelineName, err := resolveClientForJob(ctx, r.Client, r.Cache, job)
	if err != nil {
		setCondition(&job.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "PipelineNotReady", err.Error())
		if err2 := r.Status().Update(ctx, job); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	jobName := job.Spec.JobName
	if jobName == "" {
		jobName = job.Name
	}

	concourseTeam := cl.Team(teamName)
	pipelineRef := atc.PipelineRef{Name: pipelineName}

	if job.Spec.Paused {
		if _, err := concourseTeam.PauseJob(pipelineRef, jobName); err != nil {
			log.Error(err, "pause job")
		} else {
			job.Status.Paused = true
		}
	} else {
		if _, err := concourseTeam.UnpauseJob(pipelineRef, jobName); err != nil {
			log.Error(err, "unpause job")
		} else {
			job.Status.Paused = false
		}
	}

	// Trigger a build if requested and generation changed.
	if job.Spec.TriggerBuild && job.Status.ObservedGeneration != job.Generation {
		buildCR := &concoursev1alpha1.Build{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: job.Name + "-build-",
				Namespace:    job.Namespace,
			},
			Spec: concoursev1alpha1.BuildSpec{
				JobRef: &concoursev1alpha1.LocalObjectReference{Name: job.Name},
			},
		}
		if err := ctrl.SetControllerReference(job, buildCR, r.Scheme); err != nil {
			log.Error(err, "set controller reference on build")
		} else if err := r.Create(ctx, buildCR); err != nil {
			log.Error(err, "create build CR")
		} else {
			job.Status.NextBuildName = buildCR.Name
		}
	}

	job.Status.ObservedGeneration = job.Generation
	setCondition(&job.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "")

	if err := r.Status().Update(ctx, job); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *JobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&concoursev1alpha1.Job{}).
		Named("job").
		Complete(r)
}
