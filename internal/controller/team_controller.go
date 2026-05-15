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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	Scheme *runtime.Scheme
	Cache  *concourse.Cache
}

// +kubebuilder:rbac:groups=concourse-ci.org,resources=teams,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=concourse-ci.org,resources=teams/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=concourse-ci.org,resources=teams/finalizers,verbs=update
// +kubebuilder:rbac:groups=concourse-ci.org,resources=instances,verbs=get;list;watch

func (r *TeamReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	team := &concoursev1alpha1.Team{}
	if err := r.Get(ctx, req.NamespacedName, team); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !team.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(team, teamFinalizer) {
			if err := r.deleteTeam(ctx, team); err != nil {
				log.Error(err, "delete team from Concourse")
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

	_, cl, err := resolveInstanceForTeam(ctx, r.Client, r.Cache, team)
	if err != nil {
		setCondition(&team.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "InstanceNotReady", err.Error())
		if err2 := r.Status().Update(ctx, team); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	teamName := team.Spec.TeamName
	if teamName == "" {
		teamName = team.Name
	}

	auth := buildTeamAuth(team.Spec.Roles)
	atcTeam := atc.Team{Name: teamName, Auth: auth}

	result, _, _, _, err := cl.Team(teamName).CreateOrUpdate(atcTeam)
	if err != nil {
		setCondition(&team.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionFalse, "CreateOrUpdateFailed", err.Error())
		if err2 := r.Status().Update(ctx, team); err2 != nil {
			log.Error(err2, "update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	team.Status.TeamID = result.ID
	team.Status.ObservedGeneration = team.Generation
	setCondition(&team.Status.Conditions, concoursev1alpha1.ConditionReady, metav1.ConditionTrue, "Ready", "")

	if err := r.Status().Update(ctx, team); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *TeamReconciler) deleteTeam(ctx context.Context, team *concoursev1alpha1.Team) error {
	_, cl, err := resolveInstanceForTeam(ctx, r.Client, r.Cache, team)
	if err != nil {
		return err
	}
	teamName := team.Spec.TeamName
	if teamName == "" {
		teamName = team.Name
	}
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
