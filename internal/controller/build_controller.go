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
	"strconv"
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

// BuildReconciler reconciles a Build object.
type BuildReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Cache  *concourse.Cache
}

// +kubebuilder:rbac:groups=concourse-ci.org,resources=builds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=concourse-ci.org,resources=builds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=builds/finalizers,verbs=update
// +kubebuilder:rbac:groups=concourse-ci.org,resources=jobs,verbs=get;list;watch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=pipelines,verbs=get;list;watch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=teams,verbs=get;list;watch

func (r *BuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	build := &concoursev1alpha1.Build{}
	if err := r.Get(ctx, req.NamespacedName, build); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !build.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// If build is already in a terminal state, no further action needed.
	if isTerminal(build.Status.ConcourseStatus) {
		return ctrl.Result{}, nil
	}

	if build.Spec.JobRef == nil {
		setCondition(&build.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "NoJobRef", "jobRef is required for non-one-off builds")
		_ = r.Status().Update(ctx, build)
		return ctrl.Result{}, nil
	}

	job := &concoursev1alpha1.Job{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: build.Namespace, Name: build.Spec.JobRef.Name}, job); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cl, teamName, pipelineName, err := resolveClientForJob(ctx, r.Client, r.Cache, job)
	if err != nil {
		setCondition(&build.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "ChainNotReady", err.Error())
		if err2 := r.Status().Update(ctx, build); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	jobName := job.Spec.JobName
	if jobName == "" {
		jobName = job.Name
	}

	// Trigger build if not yet started.
	if build.Status.BuildID == 0 {
		atcBuild, err := cl.Team(teamName).CreateJobBuild(atc.PipelineRef{Name: pipelineName}, jobName)
		if err != nil {
			setCondition(&build.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "TriggerFailed", err.Error())
			if err2 := r.Status().Update(ctx, build); err2 != nil {
				log.Error(err2, "update status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		build.Status.BuildID = atcBuild.ID
		build.Status.BuildName = atcBuild.Name
		build.Status.APIURL = atcBuild.APIURL
		if atcBuild.StartTime > 0 {
			t := metav1.Unix(atcBuild.StartTime, 0)
			build.Status.StartTime = &t
		}
	}

	// Handle abort request.
	if build.Spec.Abort && build.Status.BuildID != 0 {
		if err := cl.AbortBuild(strconv.Itoa(build.Status.BuildID)); err != nil {
			log.Error(err, "abort build")
		}
	}

	// Refresh build status.
	if build.Status.BuildID != 0 {
		atcBuild, found, err := cl.Build(strconv.Itoa(build.Status.BuildID))
		if err != nil {
			log.Error(err, "get build status")
		} else if found {
			build.Status.ConcourseStatus = concoursev1alpha1.BuildPhase(atcBuild.Status)
			if atcBuild.EndTime > 0 {
				t := metav1.Unix(atcBuild.EndTime, 0)
				build.Status.EndTime = &t
			}
		}
	}

	build.Status.ObservedGeneration = build.Generation
	setCondition(&build.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Tracked", "")

	if err := r.Status().Update(ctx, build); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}

	if isTerminal(build.Status.ConcourseStatus) {
		return ctrl.Result{}, nil
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func isTerminal(s concoursev1alpha1.BuildPhase) bool {
	switch s {
	case concoursev1alpha1.BuildPhaseSucceeded, concoursev1alpha1.BuildPhaseFailed,
		concoursev1alpha1.BuildPhaseErrored, concoursev1alpha1.BuildPhaseAborted:
		return true
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *BuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&concoursev1alpha1.Build{}).
		Named("build").
		Complete(r)
}
