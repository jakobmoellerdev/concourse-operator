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

const teamFinalizer = "concourse-ci.org/team-finalizer"

// TeamReconciler reconciles a Team object.
type TeamReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Cache    *concourse.Cache
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=concourse-ci.org,resources=teams,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=concourse-ci.org,resources=teams/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=teams/finalizers,verbs=update
// +kubebuilder:rbac:groups=concourse-ci.org,resources=instances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *TeamReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	team := &concoursev1alpha1.Team{}
	if err := r.Get(ctx, req.NamespacedName, team); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if team.Spec.Suspend {
		log.Info("Reconciliation is suspended for Team", "name", team.Name)
		setCondition(&team.Status.Conditions, team.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "Suspended", "Reconciliation is suspended")
		if err := r.Status().Update(ctx, team); err != nil {
			return ctrl.Result{}, fmt.Errorf("update status: %w", err)
		}
		return ctrl.Result{}, nil
	}

	if !team.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(team, teamFinalizer) {
			if err := r.deleteTeam(ctx, team); err != nil {
				log.Error(err, "delete team from Concourse")
				setCondition(&team.Status.Conditions, team.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "DeleteFailed", err.Error())
				_ = r.Status().Update(ctx, team)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			controllerutil.RemoveFinalizer(team, teamFinalizer)
			if err := r.Update(ctx, team); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(team, teamFinalizer) {
		controllerutil.AddFinalizer(team, teamFinalizer)
		if err := r.Update(ctx, team); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
	}

	// Ensure owner reference is set if same namespace
	instance := &concoursev1alpha1.Instance{}
	if err := r.Get(ctx, refKey(team.Namespace, team.Spec.InstanceRef), instance); err == nil {
		if instance.Namespace == team.Namespace {
			before := team.DeepCopy()
			if err := controllerutil.SetControllerReference(instance, team, r.Scheme); err == nil {
				if !reflect.DeepEqual(before.OwnerReferences, team.OwnerReferences) {
					if err := r.Update(ctx, team); err != nil {
						return ctrl.Result{}, fmt.Errorf("set owner reference: %w", err)
					}
				}
			}
		}
	}

	cl, err := resolveInstanceForTeam(ctx, r.Client, r.Cache, team)
	if err != nil {
		setCondition(&team.Status.Conditions, team.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "InstanceNotReady", err.Error())
		if err2 := r.Status().Update(ctx, team); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	teamName := concoursev1alpha1.ResolvedTeamName(team)
	auth := buildTeamAuth(team.Spec.Roles)
	atcTeam := atc.Team{Name: teamName, Auth: auth}

	result, _, _, warnings, err := cl.Team(teamName).CreateOrUpdate(atcTeam)
	if err != nil {
		recordEventf(r.Recorder, team, corev1.EventTypeWarning, "CreateOrUpdateFailed", "Failed to create or update team: %v", err)
		setCondition(&team.Status.Conditions, team.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "CreateOrUpdateFailed", err.Error())
		if err2 := r.Status().Update(ctx, team); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Compose WebURL
	instanceObj := &concoursev1alpha1.Instance{}
	if err := r.Get(ctx, refKey(team.Namespace, team.Spec.InstanceRef), instanceObj); err == nil {
		base := instanceObj.Status.ExternalURL
		if base == "" {
			base = instanceObj.Spec.URL
		}
		if base != "" {
			team.Status.WebURL = fmt.Sprintf("%s/teams/%s", strings.TrimSuffix(base, "/"), teamName)
		}
	}

	now := metav1.Now()
	team.Status.TeamID = int32Ptr(result.ID)
	team.Status.ResolvedName = teamName
	team.Status.LastApplied = &now
	team.Status.ObservedGeneration = team.Generation

	if len(warnings) > 0 {
		recordEventf(r.Recorder, team, corev1.EventTypeWarning, "HasWarnings", "Team sync completed with %d warning(s)", len(warnings))
		msgs := make([]string, len(warnings))
		for i, w := range warnings {
			msgs[i] = w.Message
		}
		setCondition(&team.Status.Conditions, team.Generation, concoursev1alpha1.ConditionConfigWarning, metav1.ConditionTrue, "HasWarnings",
			fmt.Sprintf("%d warning(s): %s", len(warnings), msgs[0]))
	} else {
		setCondition(&team.Status.Conditions, team.Generation, concoursev1alpha1.ConditionConfigWarning, metav1.ConditionFalse, "NoWarnings", "")
	}

	setCondition(&team.Status.Conditions, team.Generation, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "")

	if team.Status.ObservedGeneration != team.Generation {
		recordEventf(r.Recorder, team, corev1.EventTypeNormal, "CreatedOrUpdated", "Team %q synchronized successfully", teamName)
	}

	if err := r.Status().Update(ctx, team); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *TeamReconciler) deleteTeam(ctx context.Context, team *concoursev1alpha1.Team) error {
	if team.Spec.ReclaimPolicy == concoursev1alpha1.ReclaimOrphan {
		recordEventf(r.Recorder, team, corev1.EventTypeNormal, "Orphaned", "Team deleted from Kubernetes; Concourse team remains")
		return nil
	}
	teamName := concoursev1alpha1.ResolvedTeamName(team)
	if teamName == "main" && !team.Spec.AllowDestroy {
		recordEvent(r.Recorder, team, corev1.EventTypeWarning, "DeleteRefused", "Refusing to destroy reserved 'main' team")
		return fmt.Errorf("refusing to destroy reserved team %q; set spec.allowDestroy=true to override", teamName)
	}
	cl, err := resolveInstanceForTeam(ctx, r.Client, r.Cache, team)
	if err != nil {
		return err
	}
	recordEventf(r.Recorder, team, corev1.EventTypeNormal, "Deleting", "Destroying Concourse team %q", teamName)
	return cl.Team(teamName).DestroyTeam(teamName)
}

// buildTeamAuth converts role bindings to atc.TeamAuth format.
// atc.TeamAuth is map[role]map["users"|"groups"][]string.
func buildTeamAuth(roles []concoursev1alpha1.TeamRole) atc.TeamAuth {
	if len(roles) == 0 {
		return nil
	}
	auth := make(atc.TeamAuth)
	for _, role := range roles {
		if auth[role.Role] == nil {
			auth[role.Role] = map[string][]string{}
		}
		auth[role.Role]["users"] = append(auth[role.Role]["users"], role.Users...)
		auth[role.Role]["groups"] = append(auth[role.Role]["groups"], role.Groups...)
	}
	return auth
}

// SetupWithManager sets up the controller with the Manager.
func (r *TeamReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&concoursev1alpha1.Team{}).
		Named("team").
		Complete(r)
}
