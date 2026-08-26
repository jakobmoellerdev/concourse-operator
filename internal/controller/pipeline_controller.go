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
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/concourse/concourse/atc"
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

const pipelineFinalizer = "concourse-ci.org/pipeline-finalizer"

// PipelineReconciler reconciles a Pipeline object.
type PipelineReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Cache    *concourse.Cache
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=concourse-ci.org,resources=pipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=concourse-ci.org,resources=pipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=pipelines/finalizers,verbs=update
// +kubebuilder:rbac:groups=concourse-ci.org,resources=teams,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	pipeline := &concoursev1alpha1.Pipeline{}
	if err := r.Get(ctx, req.NamespacedName, pipeline); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if pipeline.Spec.Suspend {
		log.Info("Reconciliation is suspended for Pipeline", "name", pipeline.Name)
		setCondition(&pipeline.Status.Conditions, pipeline.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "Suspended", "Reconciliation is suspended")
		if err := r.Status().Update(ctx, pipeline); err != nil {
			return ctrl.Result{}, fmt.Errorf("update status: %w", err)
		}
		return ctrl.Result{}, nil
	}

	if !pipeline.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(pipeline, pipelineFinalizer) {
			if err := r.deletePipeline(ctx, pipeline); err != nil {
				log.Error(err, "delete pipeline from Concourse")
			}
			controllerutil.RemoveFinalizer(pipeline, pipelineFinalizer)
			if err := r.Update(ctx, pipeline); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(pipeline, pipelineFinalizer) {
		controllerutil.AddFinalizer(pipeline, pipelineFinalizer)
		if err := r.Update(ctx, pipeline); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
	}

	// Ensure owner reference is set if same namespace
	team := &concoursev1alpha1.Team{}
	if err := r.Get(ctx, refKey(pipeline.Namespace, pipeline.Spec.TeamRef), team); err == nil {
		if team.Namespace == pipeline.Namespace {
			before := pipeline.DeepCopy()
			if err := controllerutil.SetControllerReference(team, pipeline, r.Scheme); err == nil {
				if !reflect.DeepEqual(before.OwnerReferences, pipeline.OwnerReferences) {
					if err := r.Update(ctx, pipeline); err != nil {
						return ctrl.Result{}, fmt.Errorf("set owner reference: %w", err)
					}
				}
			}
		}
	}

	cl, teamName, err := resolveClientForPipeline(ctx, r.Client, r.Cache, pipeline)
	if err != nil {
		setCondition(&pipeline.Status.Conditions, pipeline.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "TeamNotReady", err.Error())
		if err2 := r.Status().Update(ctx, pipeline); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	pipelineName := concoursev1alpha1.ResolvedPipelineName(pipeline)
	pipeline.Status.ResolvedName = pipelineName

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
				pipeline.Status.WebURL = fmt.Sprintf("%s/teams/%s/pipelines/%s", strings.TrimSuffix(base, "/"), teamName, pipelineName)
			}
		}
	}

	yamlBytes, err := r.loadPipelineConfig(ctx, pipeline)
	if err != nil {
		setCondition(&pipeline.Status.Conditions, pipeline.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "ConfigLoadFailed", err.Error())
		if err2 := r.Status().Update(ctx, pipeline); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// The config is applied to Concourse verbatim. Any per-environment values
	// are expected to be baked into config.inline by the author (e.g. kro CEL
	// at the RGD layer). This controller does no variable interpolation.
	newHash := fmt.Sprintf("%x", sha256.Sum256(yamlBytes))
	concourseTeam := cl.Team(teamName)
	pipelineRef := atc.PipelineRef{Name: pipelineName}

	if pipeline.Status.ConfigHash != newHash || pipeline.Status.PipelineID == nil {
		// Optimistic-concurrency: Concourse set-config requires the CURRENT
		// config version as a token (X-Concourse-Config-Version). Passing an
		// empty token makes the ATC unable to reconcile and it rejects the
		// save with "comparison with existing config failed during save".
		// Fetch the current version first (empty for a brand-new pipeline),
		// then set-config with it.
		currentVersion := func() string {
			if _, v, found, cfgErr := concourseTeam.PipelineConfig(pipelineRef); cfgErr == nil && found {
				return v
			}
			return ""
		}
		_, _, warnings, err := concourseTeam.CreateOrUpdatePipelineConfig(pipelineRef, currentVersion(), yamlBytes, false)
		// A version mismatch (another writer advanced the config between our
		// fetch and write) surfaces as the comparison error. Re-fetch the
		// version and retry ONCE; a persistent failure then surfaces as
		// SetPipelineFailed rather than looping.
		if err != nil && isConfigComparisonError(err) {
			log.Info("pipeline set-config version raced; re-fetching config version and retrying", "pipeline", pipelineRef.String())
			_, _, warnings, err = concourseTeam.CreateOrUpdatePipelineConfig(pipelineRef, currentVersion(), yamlBytes, false)
		}
		if err != nil {
			recordEventf(r.Recorder, pipeline, corev1.EventTypeWarning, "SetPipelineFailed", "Failed to update pipeline config: %v", err)
			setCondition(&pipeline.Status.Conditions, pipeline.Generation, concoursev1alpha1.ConditionConfigSynced, metav1.ConditionFalse, "SetPipelineFailed", err.Error())
			setCondition(&pipeline.Status.Conditions, pipeline.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "SetPipelineFailed", err.Error())
			if err2 := r.Status().Update(ctx, pipeline); err2 != nil {
				log.Error(err2, "update status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		if len(warnings) > 0 {
			recordEventf(r.Recorder, pipeline, corev1.EventTypeWarning, "HasWarnings", "Pipeline config updated with %d warning(s)", len(warnings))
			log.Info("pipeline config warnings", "warnings", warnings)
			setCondition(&pipeline.Status.Conditions, pipeline.Generation, concoursev1alpha1.ConditionConfigWarning, metav1.ConditionTrue, "HasWarnings",
				fmt.Sprintf("%d warning(s): %s", len(warnings), warnings[0].Message))
		} else {
			setCondition(&pipeline.Status.Conditions, pipeline.Generation, concoursev1alpha1.ConditionConfigWarning, metav1.ConditionFalse, "NoWarnings", "")
		}
		setCondition(&pipeline.Status.Conditions, pipeline.Generation, concoursev1alpha1.ConditionConfigSynced, metav1.ConditionTrue, "Synced", "")
		pipeline.Status.ConfigHash = newHash
	}

	// Archive state
	if pipeline.Spec.Archived {
		if _, err := concourseTeam.ArchivePipeline(pipelineRef); err != nil {
			log.Error(err, "archive pipeline")
			recordEventf(r.Recorder, pipeline, corev1.EventTypeWarning, "ArchiveFailed", "Failed to archive pipeline: %v", err)
			setCondition(&pipeline.Status.Conditions, pipeline.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "ArchiveFailed", err.Error())
			if err2 := r.Status().Update(ctx, pipeline); err2 != nil {
				return ctrl.Result{}, fmt.Errorf("update status: %w", err2)
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		pipeline.Status.Archived = boolPtr(true)
	} else {
		pipeline.Status.Archived = boolPtr(false)
	}

	if pipeline.Spec.Paused {
		if _, err := concourseTeam.PausePipeline(pipelineRef); err != nil {
			log.Error(err, "pause pipeline")
			setCondition(&pipeline.Status.Conditions, pipeline.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "PauseFailed", err.Error())
			if err2 := r.Status().Update(ctx, pipeline); err2 != nil {
				return ctrl.Result{}, fmt.Errorf("update status: %w", err2)
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		pipeline.Status.Paused = boolPtr(true)
	} else {
		if _, err := concourseTeam.UnpausePipeline(pipelineRef); err != nil {
			log.Error(err, "unpause pipeline")
			setCondition(&pipeline.Status.Conditions, pipeline.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "UnpauseFailed", err.Error())
			if err2 := r.Status().Update(ctx, pipeline); err2 != nil {
				return ctrl.Result{}, fmt.Errorf("update status: %w", err2)
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		pipeline.Status.Paused = boolPtr(false)
	}

	if pipeline.Spec.Exposed {
		if _, err := concourseTeam.ExposePipeline(pipelineRef); err != nil {
			log.Error(err, "expose pipeline")
			setCondition(&pipeline.Status.Conditions, pipeline.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "ExposeFailed", err.Error())
			if err2 := r.Status().Update(ctx, pipeline); err2 != nil {
				return ctrl.Result{}, fmt.Errorf("update status: %w", err2)
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		pipeline.Status.Exposed = boolPtr(true)
	} else {
		if _, err := concourseTeam.HidePipeline(pipelineRef); err != nil {
			log.Error(err, "hide pipeline")
			setCondition(&pipeline.Status.Conditions, pipeline.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "HideFailed", err.Error())
			if err2 := r.Status().Update(ctx, pipeline); err2 != nil {
				return ctrl.Result{}, fmt.Errorf("update status: %w", err2)
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		pipeline.Status.Exposed = boolPtr(false)
	}

	atcPipeline, found, err := concourseTeam.Pipeline(pipelineRef)
	if err == nil && found {
		pipeline.Status.PipelineID = int32Ptr(atcPipeline.ID)
		pipeline.Status.GroupCount = int32Ptr(len(atcPipeline.Groups))
		if atcPipeline.LastUpdated > 0 {
			t := metav1.Unix(atcPipeline.LastUpdated, 0)
			pipeline.Status.LastUpdated = &t
		}
		pipeline.Status.Paused = boolPtr(atcPipeline.Paused)
		pipeline.Status.Exposed = boolPtr(atcPipeline.Public)
		pipeline.Status.Archived = boolPtr(atcPipeline.Archived)
		pipeline.Status.PausedBy = atcPipeline.PausedBy
		if atcPipeline.PausedAt > 0 {
			pat := metav1.Unix(atcPipeline.PausedAt, 0)
			pipeline.Status.PausedAt = &pat
		}

		// Map groups
		groups := make([]concoursev1alpha1.PipelineGroupStatus, 0, len(atcPipeline.Groups))
		for _, g := range atcPipeline.Groups {
			groups = append(groups, concoursev1alpha1.PipelineGroupStatus{
				Name:      g.Name,
				Jobs:      g.Jobs,
				Resources: g.Resources,
			})
		}
		pipeline.Status.Groups = groups
	}

	if rtypes, _, rterr := cl.Team(teamName).ResourceTypes(pipelineRef); rterr == nil {
		observed := make([]concoursev1alpha1.PipelineResourceTypeStatus, 0, len(rtypes))
		for _, rt := range rtypes {
			observed = append(observed, concoursev1alpha1.PipelineResourceTypeStatus{
				Name:       rt.Name,
				Type:       rt.Type,
				Privileged: rt.Privileged,
				Tags:       rt.Tags,
			})
		}
		pipeline.Status.ResourceTypes = observed
	}

	if jobs, jerr := concourseTeam.ListJobs(pipelineRef); jerr == nil {
		observed := make([]concoursev1alpha1.PipelineJobStatus, 0, len(jobs))
		for _, j := range jobs {
			st := concoursev1alpha1.PipelineJobStatus{Name: j.Name, Paused: j.Paused}
			if j.FinishedBuild != nil {
				st.LastBuildStatus = concoursev1alpha1.BuildPhase(j.FinishedBuild.Status)
			}
			observed = append(observed, st)
		}
		pipeline.Status.Jobs = observed
	}

	if resources, rerr := concourseTeam.ListResources(pipelineRef); rerr == nil {
		observed := make([]concoursev1alpha1.PipelineResourceStatus, 0, len(resources))
		for _, res := range resources {
			observed = append(observed, concoursev1alpha1.PipelineResourceStatus{
				Name:   res.Name,
				Type:   res.Type,
				Pinned: res.PinComment != "" || len(res.PinnedVersion) > 0,
			})
		}
		pipeline.Status.Resources = observed
	}

	pipeline.Status.ObservedGeneration = pipeline.Generation
	setCondition(&pipeline.Status.Conditions, pipeline.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "")

	if pipeline.Status.ObservedGeneration != pipeline.Generation {
		recordEventf(r.Recorder, pipeline, corev1.EventTypeNormal, "ConfigSynced", "Pipeline %q synchronized successfully", pipelineName)
	}

	if err := r.Status().Update(ctx, pipeline); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *PipelineReconciler) loadPipelineConfig(ctx context.Context, pipeline *concoursev1alpha1.Pipeline) ([]byte, error) {
	cfg := pipeline.Spec.Config
	if cfg.Inline != "" {
		return []byte(cfg.Inline), nil
	}
	if cfg.ConfigMapRef != nil {
		cm := &corev1.ConfigMap{}
		key := client.ObjectKey{Namespace: pipeline.Namespace, Name: cfg.ConfigMapRef.Name}
		if err := r.Get(ctx, key, cm); err != nil {
			return nil, fmt.Errorf("get configmap %s: %w", cfg.ConfigMapRef.Name, err)
		}
		val, ok := cm.Data[cfg.ConfigMapRef.Key]
		if !ok {
			return nil, fmt.Errorf("key %q not found in configmap %s", cfg.ConfigMapRef.Key, cfg.ConfigMapRef.Name)
		}
		return []byte(val), nil
	}
	return nil, fmt.Errorf("pipeline config must specify either inline or configMapRef")
}

// isConfigComparisonError reports whether an error from set-config is the
// Concourse ATC "comparison with existing config failed during save" error,
// which indicates a stale/empty config-version token (optimistic-concurrency
// mismatch). Used to gate a single re-fetch-and-retry of set-config.
func isConfigComparisonError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "comparison with existing config failed")
}

func (r *PipelineReconciler) deletePipeline(ctx context.Context, pipeline *concoursev1alpha1.Pipeline) error {
	if pipeline.Spec.ReclaimPolicy == concoursev1alpha1.ReclaimOrphan {
		recordEvent(r.Recorder, pipeline, corev1.EventTypeNormal, "Orphaned", "Pipeline deleted from Kubernetes; Concourse pipeline remains")
		return nil
	}
	cl, teamName, err := resolveClientForPipeline(ctx, r.Client, r.Cache, pipeline)
	if err != nil {
		return err
	}
	pipelineName := concoursev1alpha1.ResolvedPipelineName(pipeline)
	recordEventf(r.Recorder, pipeline, corev1.EventTypeNormal, "Deleting", "Deleting Concourse pipeline %q", pipelineName)
	_, err = cl.Team(teamName).DeletePipeline(atc.PipelineRef{Name: pipelineName})
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&concoursev1alpha1.Pipeline{}).
		Named("pipeline").
		Complete(r)
}
