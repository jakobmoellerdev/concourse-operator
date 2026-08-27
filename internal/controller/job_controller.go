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
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/concourse/concourse/atc"
	concourseapi "github.com/concourse/concourse/go-concourse/concourse"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

// JobReconciler reconciles a Job object.
type JobReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Cache    *concourse.Cache
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=concourse-ci.org,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=concourse-ci.org,resources=jobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=jobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=concourse-ci.org,resources=pipelines,verbs=get;list;watch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=teams,verbs=get;list;watch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=builds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *JobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	job := &concoursev1alpha1.Job{}
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if handled, err := r.handleJobLifecycle(ctx, job); handled {
		return ctrl.Result{}, err
	}

	cl, teamName, pipelineName, err := resolveClientForJob(ctx, r.Client, r.Cache, job)
	if err != nil {
		setCondition(&job.Status.Conditions, job.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "PipelineNotReady", err.Error())
		if err2 := r.Status().Update(ctx, job); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Ensure owner reference is set if same namespace
	pipeline := &concoursev1alpha1.Pipeline{}
	if err := r.Get(ctx, refKey(job.Namespace, job.Spec.PipelineRef), pipeline); err == nil {
		if pipeline.Namespace == job.Namespace {
			before := job.DeepCopy()
			if err := controllerutil.SetControllerReference(pipeline, job, r.Scheme); err == nil {
				if !reflect.DeepEqual(before.OwnerReferences, job.OwnerReferences) {
					if err := r.Update(ctx, job); err != nil {
						return ctrl.Result{}, fmt.Errorf("set owner reference: %w", err)
					}
				}
			}
		}
	}

	jobName := concoursev1alpha1.ResolvedJobName(job)
	job.Status.ResolvedName = jobName

	// Compose WebURL
	pipelineObj := &concoursev1alpha1.Pipeline{}
	if err := r.Get(ctx, refKey(job.Namespace, job.Spec.PipelineRef), pipelineObj); err == nil {
		teamObj := &concoursev1alpha1.Team{}
		if err := r.Get(ctx, refKey(pipelineObj.Namespace, pipelineObj.Spec.TeamRef), teamObj); err == nil {
			instanceObj := &concoursev1alpha1.Instance{}
			if err := r.Get(ctx, refKey(teamObj.Namespace, teamObj.Spec.InstanceRef), instanceObj); err == nil {
				base := instanceObj.Status.ExternalURL
				if base == "" {
					base = instanceObj.Spec.URL
				}
				if base != "" {
					job.Status.WebURL = fmt.Sprintf("%s/teams/%s/pipelines/%s/jobs/%s", strings.TrimSuffix(base, "/"), teamName, pipelineName, jobName)
				}
			}
		}
	}

	concourseTeam := cl.Team(teamName)
	pipelineRef := atc.PipelineRef{Name: pipelineName}

	if handled, res, err := r.applyJobPause(ctx, job, concourseTeam, pipelineRef, jobName); handled {
		return res, err
	}

	atcJob, found, err := concourseTeam.Job(pipelineRef, jobName)
	if err != nil {
		log.Error(err, "fetch job info")
		recordEventf(r.Recorder, job, corev1.EventTypeWarning, "FetchFailed", "Failed to retrieve job details: %v", err)
		setCondition(&job.Status.Conditions, job.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "FetchFailed", err.Error())
		setCondition(&job.Status.Conditions, job.Generation, concoursev1alpha1.ConditionLastBuildSucceeded, metav1.ConditionUnknown, "FetchFailed", err.Error())
		if err2 := r.Status().Update(ctx, job); err2 != nil {
			return ctrl.Result{}, fmt.Errorf("update status: %w", err2)
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	if !found {
		recordEventf(r.Recorder, job, corev1.EventTypeWarning, "JobNotFound", "Job %q not found in pipeline %q", jobName, pipelineName)
		setCondition(&job.Status.Conditions, job.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "JobNotFound", fmt.Sprintf("job %q not found in pipeline %q", jobName, pipelineName))
		setCondition(&job.Status.Conditions, job.Generation, concoursev1alpha1.ConditionLastBuildSucceeded, metav1.ConditionUnknown, "NoBuild", "job not found")
		if err2 := r.Status().Update(ctx, job); err2 != nil {
			return ctrl.Result{}, fmt.Errorf("update status: %w", err2)
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	job.Status.JobID = int32Ptr(atcJob.ID)
	job.Status.HasNewInputs = new(atcJob.HasNewInputs)
	job.Status.Groups = atcJob.Groups
	job.Status.DisableManualTrigger = new(atcJob.DisableManualTrigger)
	job.Status.PausedBy = atcJob.PausedBy
	if atcJob.PausedAt > 0 {
		pa := metav1.Unix(atcJob.PausedAt, 0)
		job.Status.PausedAt = &pa
	}
	if atcJob.NextBuild != nil {
		job.Status.NextBuildID = int32Ptr(atcJob.NextBuild.ID)
		job.Status.NextBuildStatus = concoursev1alpha1.BuildPhase(atcJob.NextBuild.Status)
		job.Status.NextBuildName = atcJob.NextBuild.Name
	} else {
		job.Status.NextBuildID = nil
		job.Status.NextBuildStatus = ""
		job.Status.NextBuildName = ""
	}

	// Map Inputs
	inputs := make([]concoursev1alpha1.JobInputStatus, 0, len(atcJob.Inputs))
	for _, inp := range atcJob.Inputs {
		inputs = append(inputs, concoursev1alpha1.JobInputStatus{
			Name:     inp.Name,
			Resource: inp.Resource,
			Trigger:  inp.Trigger,
			Passed:   inp.Passed,
		})
	}
	job.Status.Inputs = inputs

	// Map Outputs
	outputs := make([]concoursev1alpha1.JobOutputStatus, 0, len(atcJob.Outputs))
	for _, out := range atcJob.Outputs {
		outputs = append(outputs, concoursev1alpha1.JobOutputStatus{
			Name:     out.Name,
			Resource: out.Resource,
		})
	}
	job.Status.Outputs = outputs

	job.Status.Paused = new(atcJob.Paused)

	if atcJob.FinishedBuild != nil {
		fb := atcJob.FinishedBuild
		job.Status.LastBuildID = int32Ptr(fb.ID)
		job.Status.LastBuildStatus = concoursev1alpha1.BuildPhase(fb.Status)
		if fb.EndTime > 0 {
			t := metav1.Unix(fb.EndTime, 0)
			job.Status.LastBuildTime = &t
		}
		if concoursev1alpha1.BuildPhase(fb.Status) == concoursev1alpha1.BuildPhaseSucceeded {
			setCondition(&job.Status.Conditions, job.Generation, concoursev1alpha1.ConditionLastBuildSucceeded, metav1.ConditionTrue, "Succeeded", "")
		} else {
			setCondition(&job.Status.Conditions, job.Generation, concoursev1alpha1.ConditionLastBuildSucceeded, metav1.ConditionFalse, mapPhaseToReason(string(fb.Status)), "")
		}
	} else {
		setCondition(&job.Status.Conditions, job.Generation, concoursev1alpha1.ConditionLastBuildSucceeded, metav1.ConditionUnknown, "NoBuild", "no finished build yet")
	}

	if err := r.pruneBuilds(ctx, job); err != nil {
		log.Error(err, "prune build history")
	}

	job.Status.ObservedGeneration = job.Generation
	setCondition(&job.Status.Conditions, job.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "")

	if err := r.Status().Update(ctx, job); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

func mapPhaseToReason(phase string) string {
	switch phase {
	case "pending":
		return "Pending"
	case "started":
		return "Started"
	case "succeeded":
		return "Succeeded"
	case "failed":
		return "Failed"
	case "errored":
		return "Errored"
	case "aborted":
		return "Aborted"
	default:
		return "Unknown"
	}
}

// handleJobLifecycle processes suspend and deletion short-circuits. It returns
// handled=true when the caller should return the provided result immediately.
func (r *JobReconciler) handleJobLifecycle(ctx context.Context, job *concoursev1alpha1.Job) (bool, error) {
	log := logf.FromContext(ctx)

	if job.Spec.Suspend {
		log.Info("Reconciliation is suspended for Job", "name", job.Name)
		setCondition(&job.Status.Conditions, job.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "Suspended", "Reconciliation is suspended")
		if err := r.Status().Update(ctx, job); err != nil {
			return true, fmt.Errorf("update status: %w", err)
		}
		return true, nil
	}

	if !job.DeletionTimestamp.IsZero() {
		return true, nil
	}

	return false, nil
}

// applyJobPause pauses or unpauses the job in Concourse according to spec.paused.
// It returns handled=true when the caller should return the provided result
// immediately.
func (r *JobReconciler) applyJobPause(ctx context.Context, job *concoursev1alpha1.Job, concourseTeam concourseapi.Team, pipelineRef atc.PipelineRef, jobName string) (bool, ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if job.Spec.Paused {
		if _, err := concourseTeam.PauseJob(pipelineRef, jobName); err != nil {
			log.Error(err, "pause job")
			recordEventf(r.Recorder, job, corev1.EventTypeWarning, "PauseFailed", "Failed to pause job: %v", err)
			setCondition(&job.Status.Conditions, job.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "PauseFailed", err.Error())
			if err2 := r.Status().Update(ctx, job); err2 != nil {
				return true, ctrl.Result{}, fmt.Errorf("update status: %w", err2)
			}
			return true, ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		job.Status.Paused = new(true)
	} else {
		if _, err := concourseTeam.UnpauseJob(pipelineRef, jobName); err != nil {
			log.Error(err, "unpause job")
			recordEventf(r.Recorder, job, corev1.EventTypeWarning, "UnpauseFailed", "Failed to unpause job: %v", err)
			setCondition(&job.Status.Conditions, job.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "UnpauseFailed", err.Error())
			if err2 := r.Status().Update(ctx, job); err2 != nil {
				return true, ctrl.Result{}, fmt.Errorf("update status: %w", err2)
			}
			return true, ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		job.Status.Paused = new(false)
	}

	return false, ctrl.Result{}, nil
}

func (r *JobReconciler) pruneBuilds(ctx context.Context, job *concoursev1alpha1.Job) error {
	list := &concoursev1alpha1.BuildList{}
	if err := r.List(ctx, list, client.InNamespace(job.Namespace)); err != nil {
		return err
	}
	var owned []concoursev1alpha1.Build
	for i := range list.Items {
		b := list.Items[i]
		if !metav1.IsControlledBy(&b, job) {
			continue
		}
		owned = append(owned, b)
	}
	successLimit := int32(3)
	if job.Spec.SuccessfulBuildsHistoryLimit != nil {
		successLimit = *job.Spec.SuccessfulBuildsHistoryLimit
	}
	failLimit := int32(3)
	if job.Spec.FailedBuildsHistoryLimit != nil {
		failLimit = *job.Spec.FailedBuildsHistoryLimit
	}

	var succeeded, failed []concoursev1alpha1.Build
	now := time.Now()
	for i := range owned {
		b := owned[i]
		if b.Spec.TTLSecondsAfterFinished != nil && isTerminal(b.Status.ConcourseStatus) && b.Status.EndTime != nil {
			deadline := b.Status.EndTime.Add(time.Duration(*b.Spec.TTLSecondsAfterFinished) * time.Second)
			if now.After(deadline) {
				if err := r.Delete(ctx, &b); err != nil {
					return err
				}
				continue
			}
		}
		switch b.Status.ConcourseStatus {
		case concoursev1alpha1.BuildPhaseSucceeded:
			succeeded = append(succeeded, b)
		case concoursev1alpha1.BuildPhaseFailed, concoursev1alpha1.BuildPhaseErrored, concoursev1alpha1.BuildPhaseAborted:
			failed = append(failed, b)
		}
	}
	sort.Slice(succeeded, func(i, j int) bool {
		return succeeded[i].CreationTimestamp.After(succeeded[j].CreationTimestamp.Time)
	})
	sort.Slice(failed, func(i, j int) bool {
		return failed[i].CreationTimestamp.After(failed[j].CreationTimestamp.Time)
	})
	for i := int(successLimit); i < len(succeeded); i++ {
		if err := r.Delete(ctx, &succeeded[i]); err != nil {
			return err
		}
	}
	for i := int(failLimit); i < len(failed); i++ {
		if err := r.Delete(ctx, &failed[i]); err != nil {
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *JobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&concoursev1alpha1.Job{}).
		Owns(&concoursev1alpha1.Build{}).
		Named("job").
		Complete(r)
}
