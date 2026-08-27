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
	"time"

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

// WorkerReconciler reconciles a Worker object.
type WorkerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Cache    *concourse.Cache
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=concourse-ci.org,resources=workers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=concourse-ci.org,resources=workers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=workers/finalizers,verbs=update
// +kubebuilder:rbac:groups=concourse-ci.org,resources=instances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *WorkerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	worker := &concoursev1alpha1.Worker{}
	if err := r.Get(ctx, req.NamespacedName, worker); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if worker.Spec.Suspend {
		log.Info("Reconciliation is suspended for Worker", "name", worker.Name)
		setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "Suspended", "Reconciliation is suspended")
		if err := r.Status().Update(ctx, worker); err != nil {
			return ctrl.Result{}, fmt.Errorf("update status: %w", err)
		}
		return ctrl.Result{}, nil
	}

	if !worker.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	instance := &concoursev1alpha1.Instance{}
	if err := r.Get(ctx, refKey(worker.Namespace, worker.Spec.InstanceRef), instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure owner reference is set if same namespace
	if instance.Namespace == worker.Namespace {
		before := worker.DeepCopy()
		if err := controllerutil.SetControllerReference(instance, worker, r.Scheme); err == nil {
			if !reflect.DeepEqual(before.OwnerReferences, worker.OwnerReferences) {
				if err := r.Update(ctx, worker); err != nil {
					return ctrl.Result{}, fmt.Errorf("set owner reference: %w", err)
				}
			}
		}
	}

	if err := namespaceAllowed(instance, worker.Namespace); err != nil {
		setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "NamespaceNotAllowed", err.Error())
		_ = r.Status().Update(ctx, worker)
		return ctrl.Result{}, nil
	}
	if !isReady(instance.Status.Conditions) {
		setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "InstanceNotReady", "instance is not ready")
		_ = r.Status().Update(ctx, worker)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	cl, err := r.Cache.GetOrBuild(ctx, r.Client, instance)
	if err != nil {
		setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "ClientFailed", err.Error())
		_ = r.Status().Update(ctx, worker)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	workerName := concoursev1alpha1.ResolvedWorkerName(worker)
	worker.Status.ResolvedName = workerName

	lifecycle := worker.Spec.Lifecycle
	if lifecycle == "" {
		lifecycle = concoursev1alpha1.WorkerLifecycleRunning
	}

	switch lifecycle {
	case concoursev1alpha1.WorkerLifecycleDraining:
		if err := cl.LandWorker(workerName); err != nil {
			log.Error(err, "land worker")
			recordEventf(r.Recorder, worker, corev1.EventTypeWarning, "DrainFailed", "Failed to land worker: %v", err)
			setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "TransitionFailed", err.Error())
			_ = r.Status().Update(ctx, worker)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		recordEventf(r.Recorder, worker, corev1.EventTypeNormal, "Draining", "Requested land (graceful drain) for worker %s", workerName)
	case concoursev1alpha1.WorkerLifecycleRemoved:
		if err := cl.PruneWorker(workerName); err != nil {
			log.Error(err, "prune worker")
			recordEventf(r.Recorder, worker, corev1.EventTypeWarning, "PruneFailed", "Failed to prune worker: %v", err)
			setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "TransitionFailed", err.Error())
			_ = r.Status().Update(ctx, worker)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		recordEventf(r.Recorder, worker, corev1.EventTypeNormal, "Removing", "Requested prune for worker %s", workerName)
	}

	workers, err := cl.ListWorkers()
	if err != nil {
		log.Error(err, "list workers")
		setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "ListFailed", err.Error())
		if err2 := r.Status().Update(ctx, worker); err2 != nil {
			return ctrl.Result{}, fmt.Errorf("update status: %w", err2)
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	found := false
	for _, w := range workers {
		if w.Name != workerName {
			continue
		}
		found = true
		worker.Status.Phase = concoursev1alpha1.WorkerPhase(w.State)
		worker.Status.Platform = w.Platform
		worker.Status.Tags = w.Tags
		worker.Status.ActiveContainers = int32Ptr(w.ActiveContainers)
		worker.Status.ActiveVolumes = int32Ptr(w.ActiveVolumes)
		worker.Status.ActiveTasks = int32Ptr(w.ActiveTasks)
		worker.Status.Version = w.Version
		worker.Status.Ephemeral = w.Ephemeral
		worker.Status.Team = w.Team
		if w.StartTime > 0 {
			t := metav1.Unix(w.StartTime, 0)
			worker.Status.StartTime = &t
		}

		// Map ResourceTypes
		rtypes := make([]concoursev1alpha1.WorkerResourceTypeStatus, 0, len(w.ResourceTypes))
		for _, rt := range w.ResourceTypes {
			rtypes = append(rtypes, concoursev1alpha1.WorkerResourceTypeStatus{
				Type:       rt.Type,
				Version:    rt.Version,
				Privileged: rt.Privileged,
			})
		}
		worker.Status.ResourceTypes = rtypes

		if w.State == string(concoursev1alpha1.WorkerPhaseStalled) {
			recordEventf(r.Recorder, worker, corev1.EventTypeWarning, "Stalled", "Worker %s has stalled", workerName)
			setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionStalled, metav1.ConditionTrue, "Stalled", "worker is stalled")
		} else {
			setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionStalled, metav1.ConditionFalse, "NotStalled", "")
		}

		if workerConverged(lifecycle, concoursev1alpha1.WorkerPhase(w.State)) {
			setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionStateTransitioning, metav1.ConditionFalse, "Converged", "")
		} else {
			setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionStateTransitioning, metav1.ConditionTrue, "TransitionPending",
				fmt.Sprintf("phase %q, lifecycle %q", w.State, lifecycle))
		}
		break
	}

	if !found {
		worker.Status.Phase = concoursev1alpha1.WorkerPhaseMissing
		setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "WorkerNotFound", fmt.Sprintf("worker %q not registered", workerName))
		if lifecycle == concoursev1alpha1.WorkerLifecycleRemoved {
			setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionStateTransitioning, metav1.ConditionFalse, "Converged", "")
			setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Removed", "")
		}
		worker.Status.ObservedGeneration = worker.Generation
		if err := r.Status().Update(ctx, worker); err != nil {
			return ctrl.Result{}, fmt.Errorf("update status: %w", err)
		}
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	worker.Status.ObservedGeneration = worker.Generation
	setCondition(&worker.Status.Conditions, worker.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "")

	if err := r.Status().Update(ctx, worker); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func workerConverged(lifecycle concoursev1alpha1.WorkerLifecycle, phase concoursev1alpha1.WorkerPhase) bool {
	switch lifecycle {
	case concoursev1alpha1.WorkerLifecycleRunning:
		return phase == concoursev1alpha1.WorkerPhaseRunning
	case concoursev1alpha1.WorkerLifecycleDraining:
		return phase == concoursev1alpha1.WorkerPhaseLanding || phase == concoursev1alpha1.WorkerPhaseLanded
	case concoursev1alpha1.WorkerLifecycleRemoved:
		return phase == concoursev1alpha1.WorkerPhaseMissing
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&concoursev1alpha1.Worker{}).
		Named("worker").
		Complete(r)
}
