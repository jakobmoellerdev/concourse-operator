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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

const instanceFinalizer = "concourse-ci.org/instance-finalizer"

// InstanceReconciler reconciles a Instance object.
type InstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Cache  *concourse.Cache
}

// +kubebuilder:rbac:groups=concourse-ci.org,resources=instances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=concourse-ci.org,resources=instances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=instances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *InstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	instance := &concoursev1alpha1.Instance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !instance.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(instance, instanceFinalizer) {
			r.Cache.Evict(instance)
			controllerutil.RemoveFinalizer(instance, instanceFinalizer)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(instance, instanceFinalizer) {
		controllerutil.AddFinalizer(instance, instanceFinalizer)
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
	}

	cl, err := r.Cache.GetOrBuild(ctx, r.Client, instance)
	if err != nil {
		setCondition(&instance.Status.Conditions, concoursev1alpha1.ConditionAuthenticated, metav1.ConditionFalse, "AuthFailed", err.Error())
		setCondition(&instance.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "AuthFailed", err.Error())
		if err2 := r.Status().Update(ctx, instance); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	info, err := cl.GetInfo()
	if err != nil {
		setCondition(&instance.Status.Conditions, concoursev1alpha1.ConditionAuthenticated, metav1.ConditionFalse, "InfoFailed", err.Error())
		setCondition(&instance.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "InfoFailed", err.Error())
		if err2 := r.Status().Update(ctx, instance); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	workers, err := cl.ListWorkers()
	if err != nil {
		log.Error(err, "list workers")
	}

	instance.Status.Version = info.Version
	instance.Status.WorkerVersion = info.WorkerVersion
	instance.Status.ClusterName = info.ClusterName
	instance.Status.ExternalURL = info.ExternalURL
	instance.Status.WorkerCount = len(workers)
	instance.Status.ObservedGeneration = instance.Generation

	var stalled, running int
	for _, w := range workers {
		switch w.State {
		case "stalled":
			stalled++
		case "running":
			running++
		}
	}
	instance.Status.StalledWorkers = stalled
	instance.Status.RunningWorkers = running

	if stalled > 0 {
		setCondition(&instance.Status.Conditions, concoursev1alpha1.ConditionWorkersHealthy, metav1.ConditionFalse, "WorkersStalled",
			fmt.Sprintf("%d worker(s) stalled", stalled))
	} else {
		setCondition(&instance.Status.Conditions, concoursev1alpha1.ConditionWorkersHealthy, metav1.ConditionTrue, "AllRunning", "")
	}

	setCondition(&instance.Status.Conditions, concoursev1alpha1.ConditionAuthenticated, metav1.ConditionTrue, "Authenticated", "")
	setCondition(&instance.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "")

	if err := r.Status().Update(ctx, instance); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}

	interval := instance.Spec.Interval.Duration
	if interval == 0 {
		interval = 5 * time.Minute
	}
	return ctrl.Result{RequeueAfter: interval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *InstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&concoursev1alpha1.Instance{}).
		Named("instance").
		Complete(r)
}
