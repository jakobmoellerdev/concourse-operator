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
	goconcourse "github.com/concourse/concourse/go-concourse/concourse"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

// ResourceReconciler reconciles a Resource object.
type ResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Cache  *concourse.Cache
}

// +kubebuilder:rbac:groups=concourse-ci.org,resources=resources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=concourse-ci.org,resources=resources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=resources/finalizers,verbs=update
// +kubebuilder:rbac:groups=concourse-ci.org,resources=pipelines,verbs=get;list;watch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=teams,verbs=get;list;watch

func (r *ResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	resource := &concoursev1alpha1.Resource{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !resource.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	pipeline := &concoursev1alpha1.Pipeline{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: resource.Namespace, Name: resource.Spec.PipelineRef.Name}, pipeline); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !isReady(pipeline.Status.Conditions) {
		setCondition(&resource.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "PipelineNotReady", "pipeline is not ready")
		_ = r.Status().Update(ctx, resource)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	cl, teamName, err := resolveClientForPipeline(ctx, r.Client, r.Cache, pipeline)
	if err != nil {
		setCondition(&resource.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "ChainNotReady", err.Error())
		_ = r.Status().Update(ctx, resource)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	pipelineName := pipeline.Spec.PipelineName
	if pipelineName == "" {
		pipelineName = pipeline.Name
	}
	resourceName := resource.Spec.ResourceName
	if resourceName == "" {
		resourceName = resource.Name
	}

	concourseTeam := cl.Team(teamName)

	// Trigger resource check if interval elapsed or never checked.
	if shouldCheck(resource) {
		var version atc.Version
		if len(resource.Spec.PinnedVersion) > 0 {
			version = atc.Version(resource.Spec.PinnedVersion)
		}
		if _, _, err := concourseTeam.CheckResource(atc.PipelineRef{Name: pipelineName}, resourceName, version, false); err != nil {
			log.Error(err, "check resource")
			setCondition(&resource.Status.Conditions, concoursev1alpha1.ConditionCheckHealthy, metav1.ConditionFalse, "CheckFailed", err.Error())
		} else {
			now := metav1.Now()
			resource.Status.LastChecked = &now
			setCondition(&resource.Status.Conditions, concoursev1alpha1.ConditionCheckHealthy, metav1.ConditionTrue, "CheckSucceeded", "")
		}
	} else if resource.Status.LastChecked == nil {
		setCondition(&resource.Status.Conditions, concoursev1alpha1.ConditionCheckHealthy, metav1.ConditionUnknown, "CheckPending", "check not yet triggered")
	}

	// Fetch latest version.
	if err := r.fetchLatestVersion(concourseTeam, pipelineName, resourceName, resource); err != nil {
		log.Error(err, "fetch latest version")
	}

	resource.Status.ObservedGeneration = resource.Generation
	setCondition(&resource.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "")

	if err := r.Status().Update(ctx, resource); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}

	requeueAfter := 5 * time.Minute
	if resource.Spec.CheckInterval != nil {
		requeueAfter = resource.Spec.CheckInterval.Duration
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func shouldCheck(resource *concoursev1alpha1.Resource) bool {
	if resource.Spec.CheckInterval == nil {
		return false
	}
	if resource.Status.LastChecked == nil {
		return true
	}
	return time.Since(resource.Status.LastChecked.Time) >= resource.Spec.CheckInterval.Duration
}

func (r *ResourceReconciler) fetchLatestVersion(team goconcourse.Team, pipelineName, resourceName string, resource *concoursev1alpha1.Resource) error {
	versions, _, found, err := team.ResourceVersions(atc.PipelineRef{Name: pipelineName}, resourceName, goconcourse.Page{Limit: 1}, nil)
	if err != nil {
		return err
	}
	if !found || len(versions) == 0 {
		return nil
	}
	latest := versions[0]
	resource.Status.LatestVersion = map[string]string(latest.Version)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&concoursev1alpha1.Resource{}).
		Named("resource").
		Complete(r)
}
