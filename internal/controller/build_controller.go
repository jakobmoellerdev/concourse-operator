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
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/concourse/concourse/atc"
	atcevent "github.com/concourse/concourse/atc/event"
	concourseapi "github.com/concourse/concourse/go-concourse/concourse"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			if atcBuild.CreatedBy != nil {
				build.Status.CreatedBy = *atcBuild.CreatedBy
			}
		}
	}

	if build.Status.StartTime != nil && build.Status.EndTime != nil {
		d := build.Status.EndTime.Sub(build.Status.StartTime.Time)
		build.Status.Duration = &metav1.Duration{Duration: d}
	}

	if isTerminal(build.Status.ConcourseStatus) {
		setCondition(&build.Status.Conditions, concoursev1alpha1.ConditionComplete, metav1.ConditionTrue, string(build.Status.ConcourseStatus), "")
	} else if build.Status.BuildID != 0 {
		setCondition(&build.Status.Conditions, concoursev1alpha1.ConditionComplete, metav1.ConditionFalse, "InProgress", "")
	} else {
		setCondition(&build.Status.Conditions, concoursev1alpha1.ConditionComplete, metav1.ConditionUnknown, "Pending", "build not yet triggered")
	}

	build.Status.ObservedGeneration = build.Generation
	setCondition(&build.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Tracked", "")

	if err := r.Status().Update(ctx, build); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}

	if isTerminal(build.Status.ConcourseStatus) {
		return ctrl.Result{}, nil
	}

	// Start an SSE watcher goroutine so we get enqueued immediately when
	// Concourse finishes the build, rather than waiting for the poll interval.
	if build.Status.BuildID != 0 {
		key := types.NamespacedName{Namespace: build.Namespace, Name: build.Name}
		if _, loaded := r.watchers.LoadOrStore(key.String(), struct{}{}); !loaded {
			go func() {
				defer r.watchers.Delete(key.String())
				r.watchBuildEvents(context.Background(), cl, build.Status.BuildID, key)
			}()
		}
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
	defer stream.Close()

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

// SetupWithManager sets up the controller with the Manager.
func (r *BuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.eventCh = make(chan ctrlevent.TypedGenericEvent[*concoursev1alpha1.Build], 64)
	return ctrl.NewControllerManagedBy(mgr).
		For(&concoursev1alpha1.Build{}).
		WatchesRawSource(source.Channel(r.eventCh, &handler.TypedEnqueueRequestForObject[*concoursev1alpha1.Build]{})).
		Named("build").
		Complete(r)
}
