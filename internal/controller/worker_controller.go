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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

// WorkerReconciler reconciles a Worker object.
type WorkerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Cache  *concourse.Cache
}

// +kubebuilder:rbac:groups=concourse-ci.org,resources=workers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=concourse-ci.org,resources=workers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=workers/finalizers,verbs=update
// +kubebuilder:rbac:groups=concourse-ci.org,resources=instances,verbs=get;list;watch

func (r *WorkerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	worker := &concoursev1alpha1.Worker{}
	if err := r.Get(ctx, req.NamespacedName, worker); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !worker.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	instance := &concoursev1alpha1.Instance{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: worker.Namespace, Name: worker.Spec.InstanceRef.Name}, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !isReady(instance.Status.Conditions) {
		setCondition(&worker.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "InstanceNotReady", "instance is not ready")
		_ = r.Status().Update(ctx, worker)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	cl, err := r.Cache.GetOrBuild(ctx, r.Client, instance)
	if err != nil {
		setCondition(&worker.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "ClientFailed", err.Error())
		_ = r.Status().Update(ctx, worker)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	workerName := worker.Spec.WorkerName

	// Apply desired state transitions.
	switch worker.Spec.DesiredState {
	case concoursev1alpha1.WorkerDesiredStateLand, concoursev1alpha1.WorkerDesiredStateRetire:
		if err := cl.LandWorker(workerName); err != nil {
			log.Error(err, "land worker")
		}
	case concoursev1alpha1.WorkerDesiredStatePrune:
		if err := cl.PruneWorker(workerName); err != nil {
			log.Error(err, "prune worker")
		}
	}

	// Fetch worker state from Concourse.
	workers, err := cl.ListWorkers()
	if err != nil {
		log.Error(err, "list workers")
	} else {
		for _, w := range workers {
			if w.Name == workerName {
				worker.Status.ActualState = w.State
				worker.Status.Platform = w.Platform
				worker.Status.Tags = w.Tags
				worker.Status.ActiveContainers = w.ActiveContainers
				worker.Status.ActiveVolumes = w.ActiveVolumes
				worker.Status.Version = w.Version
				worker.Status.Ephemeral = w.Ephemeral
				worker.Status.Team = w.Team
				if w.StartTime > 0 {
					t := metav1.Unix(w.StartTime, 0)
					worker.Status.StartTime = &t
				}

				if w.State == "stalled" {
					setCondition(&worker.Status.Conditions, concoursev1alpha1.ConditionStalled, metav1.ConditionTrue, "Stalled", "worker is stalled")
				} else {
					setCondition(&worker.Status.Conditions, concoursev1alpha1.ConditionStalled, metav1.ConditionFalse, "NotStalled", "")
				}

				desiredState := string(worker.Spec.DesiredState)
				if desiredState == "" {
					desiredState = string(concoursev1alpha1.WorkerDesiredStateActive)
				}
				if w.State != desiredState {
					setCondition(&worker.Status.Conditions, concoursev1alpha1.ConditionStateTransitioning, metav1.ConditionTrue, "TransitionPending",
						fmt.Sprintf("actual %q, desired %q", w.State, desiredState))
				} else {
					setCondition(&worker.Status.Conditions, concoursev1alpha1.ConditionStateTransitioning, metav1.ConditionFalse, "Converged", "")
				}
				break
			}
		}
	}

	worker.Status.ObservedGeneration = worker.Generation
	setCondition(&worker.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "")

	if err := r.Status().Update(ctx, worker); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&concoursev1alpha1.Worker{}).
		Named("worker").
		Complete(r)
}
