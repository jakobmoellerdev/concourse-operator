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
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/concourse/concourse/atc"
	atcevent "github.com/concourse/concourse/atc/event"
	concourseapi "github.com/concourse/concourse/go-concourse/concourse"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlevent "sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

// BuildReconciler reconciles a Build object.
type BuildReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Cache    *concourse.Cache
	watchers sync.Map // build CR namespace/name → struct{} sentinel
	eventCh  chan ctrlevent.TypedGenericEvent[*concoursev1alpha1.Build]
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=concourse-ci.org,resources=builds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=concourse-ci.org,resources=builds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=builds/finalizers,verbs=update
// +kubebuilder:rbac:groups=concourse-ci.org,resources=jobs,verbs=get;list;watch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=pipelines,verbs=get;list;watch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=teams,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *BuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	build := &concoursev1alpha1.Build{}
	if err := r.Get(ctx, req.NamespacedName, build); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if handled, err := r.handleBuildLifecycle(ctx, build); handled {
		return ctrl.Result{}, err
	}

	if build.Spec.JobRef == nil {
		setCondition(&build.Status.Conditions, build.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "NoJobRef", "jobRef is required")
		_ = r.Status().Update(ctx, build)
		return ctrl.Result{}, nil
	}

	job := &concoursev1alpha1.Job{}
	if err := r.Get(ctx, refKey(build.Namespace, *build.Spec.JobRef), job); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure owner reference is set if same namespace
	if job.Namespace == build.Namespace {
		before := build.DeepCopy()
		if err := controllerutil.SetControllerReference(job, build, r.Scheme); err == nil {
			if !reflect.DeepEqual(before.OwnerReferences, build.OwnerReferences) {
				if err := r.Update(ctx, build); err != nil {
					return ctrl.Result{}, fmt.Errorf("set owner reference: %w", err)
				}
			}
		}
	}

	cl, teamName, pipelineName, err := resolveClientForJob(ctx, r.Client, r.Cache, job)
	if err != nil {
		setCondition(&build.Status.Conditions, build.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "ChainNotReady", err.Error())
		if err2 := r.Status().Update(ctx, build); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	jobName := concoursev1alpha1.ResolvedJobName(job)

	// Adopt an existing build if spec.buildID is set; otherwise create once.
	if handled, res := r.ensureBuildTriggered(ctx, build, cl, teamName, pipelineName, jobName); handled {
		return res, nil
	}

	buildID := int(ptrValue(build.Status.BuildID))

	if build.Spec.Canceled && buildID != 0 {
		if err := cl.AbortBuild(strconv.Itoa(buildID)); err != nil {
			log.Error(err, "abort build")
		} else {
			recordEventf(r.Recorder, build, corev1.EventTypeNormal, "Canceling", "Requested abort of build %d", buildID)
		}
	}

	if buildID != 0 {
		if res, done := r.observeBuild(ctx, build, cl, buildID, job, teamName, pipelineName, jobName); done {
			return res, nil
		}
	}

	commentFailed := false
	if buildID != 0 {
		commentFailed = r.reconcileComment(ctx, build, cl, teamName, pipelineName, jobName)
	}

	if build.Status.StartTime != nil && build.Status.EndTime != nil {
		d := build.Status.EndTime.Sub(build.Status.StartTime.Time)
		build.Status.Duration = &metav1.Duration{Duration: d}
	}

	if isTerminal(build.Status.ConcourseStatus) {
		setCondition(&build.Status.Conditions, build.Generation, concoursev1alpha1.ConditionComplete, metav1.ConditionTrue, mapPhaseToReason(string(build.Status.ConcourseStatus)), "")
	} else if build.Status.BuildID != nil {
		setCondition(&build.Status.Conditions, build.Generation, concoursev1alpha1.ConditionComplete, metav1.ConditionFalse, "InProgress", "")
	} else {
		setCondition(&build.Status.Conditions, build.Generation, concoursev1alpha1.ConditionComplete, metav1.ConditionUnknown, "Pending", "build not yet triggered")
	}

	build.Status.ObservedGeneration = build.Generation
	// A comment failure keeps Ready=False (already set by reconcileComment) so
	// the condition surfaces the problem; it does not fail the reconcile or
	// discard the tracked build.
	if !commentFailed {
		setCondition(&build.Status.Conditions, build.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Tracked", "")
	}

	if build.Status.ObservedGeneration != build.Generation {
		recordEventf(r.Recorder, build, corev1.EventTypeNormal, "BuildReconciled", "Build %s is in phase %s", build.Name, build.Status.ConcourseStatus)
	}

	if err := r.Status().Update(ctx, build); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}

	if commentFailed {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if isTerminal(build.Status.ConcourseStatus) {
		return ctrl.Result{}, nil
	}

	if buildID != 0 {
		key := types.NamespacedName{Namespace: build.Namespace, Name: build.Name}
		if _, loaded := r.watchers.LoadOrStore(key.String(), struct{}{}); !loaded {
			go func() {
				defer r.watchers.Delete(key.String())
				r.watchBuildEvents(context.Background(), cl, buildID, key)
			}()
		}
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// ensureBuildTriggered adopts an existing build when spec.buildID is set, or
// triggers a new Concourse build exactly once and durably persists its tracking
// fields. It returns handled=true when the caller should return the provided
// result immediately.
func (r *BuildReconciler) ensureBuildTriggered(ctx context.Context, build *concoursev1alpha1.Build, cl concourseapi.Client, teamName, pipelineName, jobName string) (bool, ctrl.Result) {
	log := logf.FromContext(ctx)

	if build.Status.BuildID == nil {
		if build.Spec.BuildID != nil {
			build.Status.BuildID = build.Spec.BuildID
			recordEventf(r.Recorder, build, corev1.EventTypeNormal, "Adopted", "Adopted existing Concourse build %d", *build.Spec.BuildID)
		} else {
			var atcBuild atc.Build
			var err error
			if build.Spec.RerunOf != "" {
				atcBuild, err = cl.Team(teamName).RerunJobBuild(atc.PipelineRef{Name: pipelineName}, jobName, build.Spec.RerunOf)
			} else {
				atcBuild, err = cl.Team(teamName).CreateJobBuild(atc.PipelineRef{Name: pipelineName}, jobName)
			}
			if err != nil {
				recordEventf(r.Recorder, build, corev1.EventTypeWarning, "TriggerFailed", "Failed to trigger Concourse build: %v", err)
				setCondition(&build.Status.Conditions, build.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "TriggerFailed", err.Error())
				if err2 := r.Status().Update(ctx, build); err2 != nil {
					log.Error(err2, "update status")
				}
				return true, ctrl.Result{RequeueAfter: 30 * time.Second}
			}
			build.Status.BuildID = int32Ptr(atcBuild.ID)
			build.Status.BuildName = atcBuild.Name
			build.Status.APIURL = atcBuild.APIURL
			if atcBuild.StartTime > 0 {
				t := metav1.Unix(atcBuild.StartTime, 0)
				build.Status.StartTime = &t
			}
			if build.Spec.RerunOf != "" {
				build.Status.RerunOf = build.Spec.RerunOf
				recordEventf(r.Recorder, build, corev1.EventTypeNormal, "Triggered", "Triggered Concourse build %s as a rerun of build %s", atcBuild.Name, build.Spec.RerunOf)
			} else {
				recordEventf(r.Recorder, build, corev1.EventTypeNormal, "Triggered", "Triggered Concourse build %s", atcBuild.Name)
			}
			// CRITICAL: persist the new BuildID to status IMMEDIATELY. If we defer
			// this to the single Status().Update at the end of Reconcile and that
			// update loses a conflict (the Build CR was modified concurrently, e.g.
			// by kro re-applying it), the next reconcile would see BuildID==nil and
			// trigger a SECOND ATC build — the duplicate-build bug. Retry on
			// conflict by re-fetching and re-applying just the build tracking
			// fields so the created build is never lost/duplicated.
			if uErr := r.persistBuildTracking(ctx, build); uErr != nil {
				log.Error(uErr, "persisting new build tracking; requeueing to avoid duplicate trigger")
				return true, ctrl.Result{RequeueAfter: 2 * time.Second}
			}
		}
	}

	return false, ctrl.Result{}
}

func isTerminal(s concoursev1alpha1.BuildPhase) bool {
	switch s {
	case concoursev1alpha1.BuildPhaseSucceeded, concoursev1alpha1.BuildPhaseFailed,
		concoursev1alpha1.BuildPhaseErrored, concoursev1alpha1.BuildPhaseAborted:
		return true
	}
	return false
}

// handleBuildLifecycle processes suspend, deletion, and terminal short-circuit.
// It returns handled=true when the caller should return the provided result
// immediately.
func (r *BuildReconciler) handleBuildLifecycle(ctx context.Context, build *concoursev1alpha1.Build) (bool, error) {
	log := logf.FromContext(ctx)

	if build.Spec.Suspend {
		log.Info("Reconciliation is suspended for Build", "name", build.Name)
		setCondition(&build.Status.Conditions, build.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "Suspended", "Reconciliation is suspended")
		if err := r.Status().Update(ctx, build); err != nil {
			return true, fmt.Errorf("update status: %w", err)
		}
		return true, nil
	}

	if !build.DeletionTimestamp.IsZero() {
		return true, nil
	}

	if isTerminal(build.Status.ConcourseStatus) {
		return true, nil
	}

	return false, nil
}

// observeBuild fetches the current ATC build status, maps build IO, and composes
// the WebURL for a build that already has a tracked buildID. It returns
// done=true when the caller should return the provided result immediately.
func (r *BuildReconciler) observeBuild(ctx context.Context, build *concoursev1alpha1.Build, cl concourseapi.Client, buildID int, job *concoursev1alpha1.Job, teamName, pipelineName, jobName string) (ctrl.Result, bool) {
	log := logf.FromContext(ctx)

	atcBuild, found, err := cl.Build(strconv.Itoa(buildID))
	if err != nil {
		log.Error(err, "get build status")
	} else if !found && !isTerminal(build.Status.ConcourseStatus) {
		// The ATC no longer has this build and it never reached a terminal
		// state. This happens when the pipeline was deleted+recreated (e.g.
		// the config-comparison self-heal) while a build was in flight: the
		// ATC drops the build with the old pipeline's rows. Reset the build
		// tracking so the create path above re-triggers a FRESH build against
		// the recreated pipeline on the next reconcile, rather than being
		// stuck pending forever pointing at a vanished build.
		log.Info("tracked ATC build vanished before completing; re-triggering", "buildID", buildID)
		recordEventf(r.Recorder, build, corev1.EventTypeWarning, "BuildVanished",
			"Concourse build %d disappeared before completing (pipeline likely recreated); re-triggering", buildID)
		build.Status.BuildID = nil
		build.Status.BuildName = ""
		build.Status.ConcourseStatus = ""
		if err2 := r.Status().Update(ctx, build); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, true
	} else if found {
		build.Status.ConcourseStatus = concoursev1alpha1.BuildPhase(atcBuild.Status)
		if atcBuild.Name != "" {
			build.Status.BuildName = atcBuild.Name
		}
		if atcBuild.APIURL != "" {
			build.Status.APIURL = atcBuild.APIURL
		}
		if atcBuild.StartTime > 0 {
			t := metav1.Unix(atcBuild.StartTime, 0)
			build.Status.StartTime = &t
		}
		if atcBuild.EndTime > 0 {
			t := metav1.Unix(atcBuild.EndTime, 0)
			build.Status.EndTime = &t
		}
		if atcBuild.CreatedBy != nil {
			build.Status.CreatedBy = *atcBuild.CreatedBy
		}
	}

	// Fetch and map BuildIO
	if bio, bfound, berr := cl.BuildResources(buildID); berr == nil && bfound {
		inputs := make([]concoursev1alpha1.BuildIO, 0, len(bio.Inputs))
		for _, inp := range bio.Inputs {
			inputs = append(inputs, concoursev1alpha1.BuildIO{
				Name:            inp.Name,
				Version:         inp.Version,
				FirstOccurrence: new(inp.FirstOccurrence),
			})
		}
		build.Status.Inputs = inputs

		outputs := make([]concoursev1alpha1.BuildIO, 0, len(bio.Outputs))
		for _, out := range bio.Outputs {
			outputs = append(outputs, concoursev1alpha1.BuildIO{
				Name:    out.Name,
				Version: out.Version,
			})
		}
		build.Status.Outputs = outputs
	}

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
				if base != "" && build.Status.BuildName != "" {
					build.Status.WebURL = fmt.Sprintf("%s/teams/%s/pipelines/%s/jobs/%s/builds/%s", strings.TrimSuffix(base, "/"), teamName, pipelineName, jobName, build.Status.BuildName)
				}
			}
		}
	}

	return ctrl.Result{}, false
}

// reconcileComment sets a comment on the build via SetJobBuildComment when
// spec.Comment differs from the last comment recorded in status. It is
// idempotent: once status.Comment matches spec.Comment, no API call is made.
// A failure to set the comment does not fail the whole reconcile — it is
// logged, surfaced as a Warning event, and the Ready condition is set False
// with reason CommentFailed so the next reconcile retries. Returns true when
// the comment failed to apply, so the caller can avoid clobbering Ready=False
// with a later Ready=True update.
func (r *BuildReconciler) reconcileComment(ctx context.Context, build *concoursev1alpha1.Build, cl concourseapi.Client, teamName, pipelineName, jobName string) bool {
	log := logf.FromContext(ctx)

	if build.Spec.Comment == "" || build.Spec.Comment == build.Status.Comment {
		return false
	}

	buildName := build.Status.BuildName
	if buildName == "" {
		return false
	}

	if _, err := cl.Team(teamName).SetJobBuildComment(atc.PipelineRef{Name: pipelineName}, jobName, buildName, build.Spec.Comment); err != nil {
		log.Error(err, "Failed to set build comment", "buildName", buildName)
		recordEventf(r.Recorder, build, corev1.EventTypeWarning, "CommentFailed", "Failed to set comment on build %s: %v", buildName, err)
		setCondition(&build.Status.Conditions, build.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "CommentFailed", err.Error())
		return true
	}

	build.Status.Comment = build.Spec.Comment
	recordEventf(r.Recorder, build, corev1.EventTypeNormal, "CommentSet", "Set comment on build %s", buildName)
	return false
}

// watchBuildEvents opens an SSE stream for buildID and sends a GenericEvent
// to r.eventCh as soon as Concourse emits a terminal status event (or on any
// stream error/EOF). The caller must ensure this runs at most once per Build CR.
func (r *BuildReconciler) watchBuildEvents(ctx context.Context, cl concourseapi.Client, buildID int, key types.NamespacedName) {
	log := logf.FromContext(ctx).WithValues("buildID", buildID, "build", key)
	enqueue := func() {
		if r.eventCh == nil {
			return
		}
		r.eventCh <- ctrlevent.TypedGenericEvent[*concoursev1alpha1.Build]{Object: &concoursev1alpha1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
		}}
	}

	stream, err := cl.BuildEvents(strconv.Itoa(buildID))
	if err != nil || stream == nil {
		if err != nil {
			log.Error(err, "open build event stream")
		}
		enqueue()
		return
	}
	defer func() { _ = stream.Close() }()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		ev, err := stream.NextEvent()
		if err != nil {
			if err != io.EOF {
				log.Error(err, "build event stream error")
			}
			enqueue()
			return
		}

		if s, ok := ev.(atcevent.Status); ok && isTerminal(concoursev1alpha1.BuildPhase(s.Status)) {
			enqueue()
			return
		}
	}
}

// persistBuildTracking writes the just-created build's tracking fields
// (BuildID/BuildName/APIURL/StartTime) to status right after CreateJobBuild,
// retrying on optimistic-lock conflicts by re-fetching the latest Build and
// re-applying only those fields. This guarantees the created ATC build ID is
// durably recorded before the next reconcile, so a lost status update can never
// cause a duplicate CreateJobBuild.
func (r *BuildReconciler) persistBuildTracking(ctx context.Context, build *concoursev1alpha1.Build) error {
	id := build.Status.BuildID
	name := build.Status.BuildName
	apiURL := build.Status.APIURL
	start := build.Status.StartTime
	rerunOf := build.Status.RerunOf
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &concoursev1alpha1.Build{}
		if err := r.Get(ctx, types.NamespacedName{Name: build.Name, Namespace: build.Namespace}, latest); err != nil {
			return err
		}
		// If another reconcile already recorded a build ID, don't clobber it.
		if latest.Status.BuildID != nil {
			build.ResourceVersion = latest.ResourceVersion
			build.Status = latest.Status
			return nil
		}
		latest.Status.BuildID = id
		latest.Status.BuildName = name
		latest.Status.APIURL = apiURL
		latest.Status.StartTime = start
		latest.Status.RerunOf = rerunOf
		if err := r.Status().Update(ctx, latest); err != nil {
			return err
		}
		build.ResourceVersion = latest.ResourceVersion
		build.Status = latest.Status
		return nil
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *BuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.eventCh = make(chan ctrlevent.TypedGenericEvent[*concoursev1alpha1.Build], 64)
	return ctrl.NewControllerManagedBy(mgr).
		For(&concoursev1alpha1.Build{}).
		WatchesRawSource(source.Channel(r.eventCh, &handler.TypedEnqueueRequestForObject[*concoursev1alpha1.Build]{})).
		Named("build").
		Complete(r)
}
