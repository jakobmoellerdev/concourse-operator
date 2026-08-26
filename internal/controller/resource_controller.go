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
	"strings"
	"time"

	"github.com/concourse/concourse/atc"
	goconcourse "github.com/concourse/concourse/go-concourse/concourse"
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

// ResourceReconciler reconciles a Resource object.
type ResourceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Cache    *concourse.Cache
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=concourse-ci.org,resources=resources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=concourse-ci.org,resources=resources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=resources/finalizers,verbs=update
// +kubebuilder:rbac:groups=concourse-ci.org,resources=pipelines,verbs=get;list;watch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=teams,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *ResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	resource := &concoursev1alpha1.Resource{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if resource.Spec.Suspend {
		log.Info("Reconciliation is suspended for Resource", "name", resource.Name)
		setCondition(&resource.Status.Conditions, resource.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "Suspended", "Reconciliation is suspended")
		if err := r.Status().Update(ctx, resource); err != nil {
			return ctrl.Result{}, fmt.Errorf("update status: %w", err)
		}
		return ctrl.Result{}, nil
	}

	if !resource.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	pipeline := &concoursev1alpha1.Pipeline{}
	if err := r.Get(ctx, refKey(resource.Namespace, resource.Spec.PipelineRef), pipeline); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure owner reference is set if same namespace
	if pipeline.Namespace == resource.Namespace {
		before := resource.DeepCopy()
		if err := controllerutil.SetControllerReference(pipeline, resource, r.Scheme); err == nil {
			if !reflect.DeepEqual(before.OwnerReferences, resource.OwnerReferences) {
				if err := r.Update(ctx, resource); err != nil {
					return ctrl.Result{}, fmt.Errorf("set owner reference: %w", err)
				}
			}
		}
	}

	if !isReady(pipeline.Status.Conditions) {
		setCondition(&resource.Status.Conditions, resource.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "PipelineNotReady", "pipeline is not ready")
		_ = r.Status().Update(ctx, resource)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	cl, teamName, err := resolveClientForPipeline(ctx, r.Client, r.Cache, pipeline)
	if err != nil {
		setCondition(&resource.Status.Conditions, resource.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "ChainNotReady", err.Error())
		_ = r.Status().Update(ctx, resource)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	pipelineName := concoursev1alpha1.ResolvedPipelineName(pipeline)
	resourceName := concoursev1alpha1.ResolvedResourceName(resource)
	resource.Status.ResolvedName = resourceName

	// Compose WebURL
	teamObj := &concoursev1alpha1.Team{}
	if err := r.Get(ctx, refKey(pipeline.Namespace, pipeline.Spec.TeamRef), teamObj); err == nil {
		instanceObj := &concoursev1alpha1.Instance{}
		if err := r.Get(ctx, refKey(teamObj.Namespace, teamObj.Spec.InstanceRef), instanceObj); err == nil {
			base := instanceObj.Status.ExternalURL
			if base == "" {
				base = instanceObj.Spec.URL
			}
			if base != "" {
				resource.Status.WebURL = fmt.Sprintf("%s/teams/%s/pipelines/%s/resources/%s", strings.TrimSuffix(base, "/"), teamName, pipelineName, resourceName)
			}
		}
	}

	concourseTeam := cl.Team(teamName)
	pipelineRef := atc.PipelineRef{Name: pipelineName}

	if err := r.syncPin(ctx, concourseTeam, pipelineRef, resourceName, resource); err != nil {
		log.Error(err, "sync pin")
		recordEventf(r.Recorder, resource, corev1.EventTypeWarning, "PinFailed", "Failed to sync pinned version: %v", err)
		setCondition(&resource.Status.Conditions, resource.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "PinFailed", err.Error())
		if err2 := r.Status().Update(ctx, resource); err2 != nil {
			return ctrl.Result{}, fmt.Errorf("update status: %w", err2)
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if shouldCheck(resource) {
		var version atc.Version
		if len(resource.Spec.PinnedVersion) > 0 {
			version = atc.Version(resource.Spec.PinnedVersion)
		}
		if _, _, err := concourseTeam.CheckResource(pipelineRef, resourceName, version, false); err != nil {
			log.Error(err, "check resource")
			recordEventf(r.Recorder, resource, corev1.EventTypeWarning, "CheckFailed", "Resource check build failed: %v", err)
			setCondition(&resource.Status.Conditions, resource.Generation, concoursev1alpha1.ConditionCheckHealthy, metav1.ConditionFalse, "CheckFailed", err.Error())
		} else {
			now := metav1.Now()
			resource.Status.LastChecked = &now
			setCondition(&resource.Status.Conditions, resource.Generation, concoursev1alpha1.ConditionCheckHealthy, metav1.ConditionTrue, "CheckSucceeded", "")
		}
	} else if resource.Status.LastChecked == nil {
		setCondition(&resource.Status.Conditions, resource.Generation, concoursev1alpha1.ConditionCheckHealthy, metav1.ConditionUnknown, "CheckPending", "check not yet triggered")
	}

	if err := r.fetchLatestVersion(concourseTeam, pipelineName, resourceName, resource); err != nil {
		log.Error(err, "fetch latest version")
	}

	resource.Status.ObservedGeneration = resource.Generation
	setCondition(&resource.Status.Conditions, resource.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "")

	if resource.Status.ObservedGeneration != resource.Generation {
		recordEventf(r.Recorder, resource, corev1.EventTypeNormal, "ResourceReconciled", "Resource %q synchronized successfully", resourceName)
	}

	if err := r.Status().Update(ctx, resource); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}

	requeueAfter := 5 * time.Minute
	if resource.Spec.CheckInterval != nil {
		requeueAfter = resource.Spec.CheckInterval.Duration
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *ResourceReconciler) syncPin(ctx context.Context, team goconcourse.Team, ref atc.PipelineRef, resourceName string, resource *concoursev1alpha1.Resource) error {
	log := logf.FromContext(ctx)
	atcRes, found, err := team.Resource(ref, resourceName)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("resource %q not found in pipeline %q", resourceName, ref.Name)
	}

	desired := resource.Spec.PinnedVersion
	actual := map[string]string(atcRes.PinnedVersion)

	if len(desired) == 0 {
		if len(actual) > 0 {
			if _, err := team.UnpinResource(ref, resourceName); err != nil {
				return fmt.Errorf("unpin resource: %w", err)
			}
			recordEventf(r.Recorder, resource, corev1.EventTypeNormal, "Unpinned", "Resource %q unpinned in Concourse", resourceName)
		}
		resource.Status.Pinned = boolPtr(false)
		resource.Status.PinnedVersionID = nil
		return nil
	}

	if desiredComment := resource.Spec.PinComment; desiredComment != "" && atcRes.PinComment != desiredComment {
		if _, err := team.SetPinComment(ref, resourceName, desiredComment); err != nil {
			log.Error(err, "set pin comment")
		}
	}

	if reflect.DeepEqual(desired, actual) && atcRes.PinnedVersion != nil {
		resource.Status.Pinned = boolPtr(true)
		return nil
	}

	versions, _, found, err := team.ResourceVersions(ref, resourceName, goconcourse.Page{Limit: 50}, atc.Version(desired))
	if err != nil {
		return fmt.Errorf("list resource versions: %w", err)
	}
	if !found {
		return fmt.Errorf("resource %q not found when listing versions", resourceName)
	}
	var match *atc.ResourceVersion
	for i := range versions {
		if reflect.DeepEqual(map[string]string(versions[i].Version), desired) {
			match = &versions[i]
			break
		}
	}
	if match == nil {
		return fmt.Errorf("pinned version %v not found for resource %q", desired, resourceName)
	}
	if _, err := team.PinResourceVersion(ref, resourceName, match.ID); err != nil {
		return fmt.Errorf("pin resource version: %w", err)
	}
	recordEventf(r.Recorder, resource, corev1.EventTypeNormal, "Pinned", "Resource %q pinned to version %v", resourceName, desired)
	resource.Status.Pinned = boolPtr(true)
	resource.Status.PinnedVersionID = int32Ptr(match.ID)
	return nil
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
