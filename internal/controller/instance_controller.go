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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

const instanceFinalizer = "concourse-ci.org/instance-finalizer"

// InstanceReconciler reconciles a Instance object.
type InstanceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Cache    *concourse.Cache
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=concourse-ci.org,resources=instances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=concourse-ci.org,resources=instances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=instances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *InstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	instance := &concoursev1alpha1.Instance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if handled, err := r.handleInstanceLifecycle(ctx, instance); handled {
		return ctrl.Result{}, err
	}

	if instance.Spec.Auth.Password == nil && instance.Spec.Auth.Token == nil {
		setCondition(&instance.Status.Conditions, instance.Generation, concoursev1alpha1.ConditionAuthenticated, metav1.ConditionFalse, "InvalidSpec", "exactly one of spec.auth.password or spec.auth.token is required")
		setCondition(&instance.Status.Conditions, instance.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "InvalidSpec", "exactly one of spec.auth.password or spec.auth.token is required")
		if err := r.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, fmt.Errorf("update status: %w", err)
		}
		return ctrl.Result{}, nil
	}

	cl, err := r.Cache.GetOrBuild(ctx, r.Client, instance)
	if err != nil {
		recordEventf(r.Recorder, instance, corev1.EventTypeWarning, "AuthFailed", "Authentication failed: %v", err)
		setCondition(&instance.Status.Conditions, instance.Generation, concoursev1alpha1.ConditionAuthenticated, metav1.ConditionFalse, "AuthFailed", err.Error())
		setCondition(&instance.Status.Conditions, instance.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "AuthFailed", err.Error())
		if err2 := r.Status().Update(ctx, instance); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	info, err := cl.GetInfo()
	if err != nil {
		recordEventf(r.Recorder, instance, corev1.EventTypeWarning, "InfoFailed", "Failed to retrieve Concourse server info: %v", err)
		setCondition(&instance.Status.Conditions, instance.Generation, concoursev1alpha1.ConditionAuthenticated, metav1.ConditionFalse, "InfoFailed", err.Error())
		setCondition(&instance.Status.Conditions, instance.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "InfoFailed", err.Error())
		if err2 := r.Status().Update(ctx, instance); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	workers, err := cl.ListWorkers()
	if err != nil {
		log.Error(err, "list workers")
		recordEventf(r.Recorder, instance, corev1.EventTypeWarning, "ListFailed", "Failed to list Concourse workers: %v", err)
		setCondition(&instance.Status.Conditions, instance.Generation, concoursev1alpha1.ConditionWorkersHealthy, metav1.ConditionUnknown, "ListFailed", err.Error())
		setCondition(&instance.Status.Conditions, instance.Generation, concoursev1alpha1.ConditionAuthenticated, metav1.ConditionTrue, "Authenticated", "")
		// Ready stays auth-only; WorkersHealthy is the inventory signal.
		setCondition(&instance.Status.Conditions, instance.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "")
		instance.Status.Version = info.Version
		instance.Status.WorkerVersion = info.WorkerVersion
		instance.Status.ClusterName = info.ClusterName
		instance.Status.ExternalURL = info.ExternalURL
		instance.Status.WebURL = info.ExternalURL
		if instance.Status.WebURL == "" {
			instance.Status.WebURL = instance.Spec.URL
		}
		instance.Status.AuthSecretGeneration = concourse.AuthSecretGeneration(ctx, r.Client, instance.Namespace, instance.Spec)
		instance.Status.ObservedGeneration = instance.Generation
		if err2 := r.Status().Update(ctx, instance); err2 != nil {
			return ctrl.Result{}, fmt.Errorf("update status: %w", err2)
		}
		return ctrl.Result{RequeueAfter: instanceInterval(instance)}, nil
	}

	// Fetch extra observable data
	if uinfo, uerr := cl.UserInfo(); uerr == nil {
		instance.Status.AuthenticatedUser = uinfo.UserName
		instance.Status.AuthenticatedAdmin = new(uinfo.IsAdmin)
	}
	if wall, werr := cl.GetWall(); werr == nil && wall.Message != "" {
		instance.Status.WallMessage = wall.Message
		setCondition(&instance.Status.Conditions, instance.Generation, "WallActive", metav1.ConditionTrue, "WallSet", wall.Message)
	} else {
		setCondition(&instance.Status.Conditions, instance.Generation, "WallActive", metav1.ConditionFalse, "NoWall", "")
	}
	if teams, terr := cl.ListTeams(); terr == nil {
		instance.Status.TeamCount = int32Ptr(len(teams))
	}
	if pipelines, perr := cl.ListPipelines(); perr == nil {
		instance.Status.PipelineCount = int32Ptr(len(pipelines))
	}

	instance.Status.Version = info.Version
	instance.Status.WorkerVersion = info.WorkerVersion
	instance.Status.ClusterName = info.ClusterName
	instance.Status.ExternalURL = info.ExternalURL
	instance.Status.WebURL = info.ExternalURL
	if instance.Status.WebURL == "" {
		instance.Status.WebURL = instance.Spec.URL
	}
	instance.Status.FeatureFlags = info.FeatureFlags
	instance.Status.WorkerCount = int32Ptr(len(workers))
	instance.Status.ObservedGeneration = instance.Generation
	instance.Status.AuthSecretGeneration = concourse.AuthSecretGeneration(ctx, r.Client, instance.Namespace, instance.Spec)

	var stalled, running, landing int
	for _, w := range workers {
		switch w.State {
		case "stalled":
			stalled++
		case "running":
			running++
		case "landing":
			landing++
		}
	}
	instance.Status.StalledWorkers = int32Ptr(stalled)
	instance.Status.RunningWorkers = int32Ptr(running)
	instance.Status.LandingWorkers = int32Ptr(landing)

	if stalled > 0 {
		recordEventf(r.Recorder, instance, corev1.EventTypeWarning, "WorkersStalled", "%d worker(s) are stalled", stalled)
		setCondition(&instance.Status.Conditions, instance.Generation, concoursev1alpha1.ConditionWorkersHealthy, metav1.ConditionFalse, "WorkersStalled",
			fmt.Sprintf("%d worker(s) stalled", stalled))
	} else {
		setCondition(&instance.Status.Conditions, instance.Generation, concoursev1alpha1.ConditionWorkersHealthy, metav1.ConditionTrue, "AllRunning", "")
	}

	setCondition(&instance.Status.Conditions, instance.Generation, concoursev1alpha1.ConditionAuthenticated, metav1.ConditionTrue, "Authenticated", "")
	setCondition(&instance.Status.Conditions, instance.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "")

	if instance.Status.ObservedGeneration != instance.Generation {
		recordEvent(r.Recorder, instance, corev1.EventTypeNormal, "ReconciliationSucceeded", "Instance reconciled successfully")
	}

	if err := r.Status().Update(ctx, instance); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}

	return ctrl.Result{RequeueAfter: instanceInterval(instance)}, nil
}

// handleInstanceLifecycle processes suspend, deletion/finalizer removal, and
// finalizer addition. It returns handled=true when the caller should return the
// provided result immediately.
func (r *InstanceReconciler) handleInstanceLifecycle(ctx context.Context, instance *concoursev1alpha1.Instance) (bool, error) {
	log := logf.FromContext(ctx)

	if instance.Spec.Suspend {
		log.Info("Reconciliation is suspended for Instance", "name", instance.Name)
		setCondition(&instance.Status.Conditions, instance.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "Suspended", "Reconciliation is suspended")
		if err := r.Status().Update(ctx, instance); err != nil {
			return true, fmt.Errorf("update status: %w", err)
		}
		return true, nil
	}

	if !instance.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(instance, instanceFinalizer) {
			r.Cache.Evict(instance)
			controllerutil.RemoveFinalizer(instance, instanceFinalizer)
			if err := r.Update(ctx, instance); err != nil {
				return true, fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return true, nil
	}

	if !controllerutil.ContainsFinalizer(instance, instanceFinalizer) {
		controllerutil.AddFinalizer(instance, instanceFinalizer)
		if err := r.Update(ctx, instance); err != nil {
			return true, fmt.Errorf("add finalizer: %w", err)
		}
	}

	return false, nil
}

func instanceInterval(instance *concoursev1alpha1.Instance) time.Duration {
	if instance.Spec.HealthProbeInterval != nil && instance.Spec.HealthProbeInterval.Duration > 0 {
		return instance.Spec.HealthProbeInterval.Duration
	}
	return 5 * time.Minute
}

// SetupWithManager sets up the controller with the Manager.
func (r *InstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&concoursev1alpha1.Instance{}).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.mapSecretToInstance)).
		Named("instance").
		Complete(r)
}

func (r *InstanceReconciler) mapSecretToInstance(ctx context.Context, obj client.Object) []ctrl.Request {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}
	list := &concoursev1alpha1.InstanceList{}
	if err := r.List(ctx, list, client.InNamespace(secret.Namespace)); err != nil {
		return nil
	}
	var reqs []ctrl.Request
	for i := range list.Items {
		inst := &list.Items[i]
		if instanceUsesSecret(inst, secret.Name) {
			reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(inst)})
		}
	}
	return reqs
}

func instanceUsesSecret(inst *concoursev1alpha1.Instance, name string) bool {
	if inst.Spec.Auth.Password != nil {
		if inst.Spec.Auth.Password.PasswordRef.Name == name {
			return true
		}
		if inst.Spec.Auth.Password.ClientSecretRef != nil && inst.Spec.Auth.Password.ClientSecretRef.Name == name {
			return true
		}
	}
	if inst.Spec.Auth.Token != nil && inst.Spec.Auth.Token.TokenRef.Name == name {
		return true
	}
	if inst.Spec.TLS != nil && inst.Spec.TLS.CARef != nil && inst.Spec.TLS.CARef.Name == name {
		return true
	}
	return false
}
