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
	"crypto/sha256"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/concourse/concourse/atc"
	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
	"github.com/jakobmoellerdev/concourse-operator/internal/concourse"
)

const pipelineFinalizer = "concourse-ci.org/pipeline-finalizer"

// PipelineReconciler reconciles a Pipeline object.
type PipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Cache  *concourse.Cache
}

// +kubebuilder:rbac:groups=concourse-ci.org,resources=pipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=concourse-ci.org,resources=pipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=pipelines/finalizers,verbs=update
// +kubebuilder:rbac:groups=concourse-ci.org,resources=teams,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	pipeline := &concoursev1alpha1.Pipeline{}
	if err := r.Get(ctx, req.NamespacedName, pipeline); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
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

	cl, teamName, err := resolveClientForPipeline(ctx, r.Client, r.Cache, pipeline)
	if err != nil {
		setCondition(&pipeline.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "TeamNotReady", err.Error())
		if err2 := r.Status().Update(ctx, pipeline); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	pipelineName := pipeline.Spec.PipelineName
	if pipelineName == "" {
		pipelineName = pipeline.Name
	}

	yamlBytes, err := r.loadPipelineConfig(ctx, pipeline)
	if err != nil {
		setCondition(&pipeline.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "ConfigLoadFailed", err.Error())
		if err2 := r.Status().Update(ctx, pipeline); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	newHash := fmt.Sprintf("%x", sha256.Sum256(yamlBytes))
	concourseTeam := cl.Team(teamName)

	if pipeline.Status.ConfigHash != newHash || pipeline.Status.PipelineID == 0 {
		pipelineRef := atc.PipelineRef{Name: pipelineName}
		_, _, warnings, err := concourseTeam.CreateOrUpdatePipelineConfig(pipelineRef, "", yamlBytes, false)
		if err != nil {
			setCondition(&pipeline.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "SetPipelineFailed", err.Error())
			if err2 := r.Status().Update(ctx, pipeline); err2 != nil {
				log.Error(err2, "update status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		if len(warnings) > 0 {
			log.Info("pipeline config warnings", "warnings", warnings)
		}
		pipeline.Status.ConfigHash = newHash
	}

	// Sync pause state.
	pipelineRef := atc.PipelineRef{Name: pipelineName}
	if pipeline.Spec.Paused {
		if _, err := concourseTeam.PausePipeline(pipelineRef); err != nil {
			log.Error(err, "pause pipeline")
		} else {
			pipeline.Status.Paused = true
		}
	} else {
		if _, err := concourseTeam.UnpausePipeline(pipelineRef); err != nil {
			log.Error(err, "unpause pipeline")
		} else {
			pipeline.Status.Paused = false
		}
	}

	// Sync expose state.
	if pipeline.Spec.Exposed {
		if _, err := concourseTeam.ExposePipeline(pipelineRef); err != nil {
			log.Error(err, "expose pipeline")
		} else {
			pipeline.Status.Exposed = true
		}
	} else {
		if _, err := concourseTeam.HidePipeline(pipelineRef); err != nil {
			log.Error(err, "hide pipeline")
		} else {
			pipeline.Status.Exposed = false
		}
	}

	// Fetch pipeline ID.
	atcPipeline, found, err := concourseTeam.Pipeline(pipelineRef)
	if err == nil && found {
		pipeline.Status.PipelineID = atcPipeline.ID
	}

	pipeline.Status.ObservedGeneration = pipeline.Generation
	setCondition(&pipeline.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "")

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

func (r *PipelineReconciler) deletePipeline(ctx context.Context, pipeline *concoursev1alpha1.Pipeline) error {
	cl, teamName, err := resolveClientForPipeline(ctx, r.Client, r.Cache, pipeline)
	if err != nil {
		return err
	}
	pipelineName := pipeline.Spec.PipelineName
	if pipelineName == "" {
		pipelineName = pipeline.Name
	}
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
