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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pgv1 "github.com/tarteo/pg-resource-operator/api/v1"
	constants "github.com/tarteo/pg-resource-operator/internal/common"
	helpers "github.com/tarteo/pg-resource-operator/internal/helpers"
	"github.com/tarteo/pg-resource-operator/internal/pg"
)

// PostgresRoleMembershipReconciler reconciles a PostgresRoleMembership object
type PostgresRoleMembershipReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pg.onestein.nl,resources=postgresrolememberships,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pg.onestein.nl,resources=postgresrolememberships/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pg.onestein.nl,resources=postgresrolememberships/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PostgresRoleMembership object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/reconcile
func (r *PostgresRoleMembershipReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("namespace", req.NamespacedName)
	log.Info("reconciling rolemembership")

	var rolemembership pgv1.PostgresRoleMembership
	if err := r.Get(ctx, req.NamespacedName, &rolemembership); err != nil {
		if errors.IsNotFound(err) {
			log.Info("rolemembership resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to find rolemembership")
		return ctrl.Result{}, err
	}

	// Apply finalizer
	if rolemembership.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&rolemembership, constants.Finalizer) {
			controllerutil.AddFinalizer(&rolemembership, constants.Finalizer)
			if err := r.Update(ctx, &rolemembership); err != nil {
				log.Error(err, "unable to update rolemembership with finalizer")
				return ctrl.Result{}, err
			}

			// Return and requeue to get fresh object
			return ctrl.Result{Requeue: true}, nil
		}
		// Set progressing status
		if changed, err := r.setProgressing(ctx, &rolemembership, "Reconciling rolemembership"); err != nil {
			log.Error(err, "unable to set progressing status")
			return ctrl.Result{}, err
		} else if changed {
			// Requeue to get fresh object with updated status
			return ctrl.Result{Requeue: true}, nil
		}
	}

	if err := r.ReconcileResource(ctx, &rolemembership); err != nil {
		log.Error(err, "unable to reconcile rolemembership")

		// Set degraded status
		if _, errStatus := r.setDegraded(ctx, &rolemembership, err.Error()); errStatus != nil {
			log.Error(errStatus, "unable to set degraded status")
			return ctrl.Result{}, errStatus
		}
		return ctrl.Result{}, err
	}

	// Remove finalizer if deletion timestamp is set
	if !rolemembership.ObjectMeta.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(&rolemembership, constants.Finalizer) {
		rolemembershipOriginal := rolemembership.DeepCopy()
		controllerutil.RemoveFinalizer(&rolemembership, constants.Finalizer)
		if err := r.Patch(ctx, &rolemembership, client.MergeFrom(rolemembershipOriginal)); err != nil {
			log.Error(err, "unable to remove finalizer from rolemembership")
			return ctrl.Result{}, err
		}

		log.Info("successfully reconciled rolemembership (deleted)")

		return ctrl.Result{}, nil
	}

	// Set ready status
	if _, err := r.setReady(ctx, &rolemembership, "Rolemembership successfully reconciled"); err != nil {
		log.Error(err, "unable to set ready status")
		return ctrl.Result{}, err
	}

	log.Info("successfully reconciled rolemembership")

	return ctrl.Result{}, nil
}

func (r *PostgresRoleMembershipReconciler) ReconcileResource(ctx context.Context, rolemembership *pgv1.PostgresRoleMembership) error {
	log := log.FromContext(ctx).WithValues("namespace", types.NamespacedName{Namespace: rolemembership.Namespace, Name: rolemembership.Name})

	handler, err := rolemembership.Spec.PostgresRef.GetPostgresHandle(ctx, r.Client, rolemembership.Namespace)
	if err != nil {
		return err
	}
	// nolint:errcheck
	defer handler.Close()

	// Resolve role and member names
	roleName, err := rolemembership.Spec.Role.GetName(ctx, r.Client, rolemembership.Namespace)
	if err != nil {
		return err
	}
	memberName, err := rolemembership.Spec.Member.GetName(ctx, r.Client, rolemembership.Namespace)
	if err != nil {
		return err
	}

	// Check if member is a member of role
	isMemberOf, err := pg.IsMemberOf(handler, roleName, memberName)
	if err != nil {
		return err
	}

	// Handle deletion
	if !rolemembership.ObjectMeta.DeletionTimestamp.IsZero() {
		if isMemberOf {
			err := pg.RevokeRole(handler, roleName, memberName)
			if err != nil {
				log.Error(err, "unable to revoke role membership")
				return err
			}
		}
		return nil
	}

	if !isMemberOf && rolemembership.Spec.Granted {
		// Grant role membership if not already a member and should be granted
		err := pg.GrantRole(
			handler,
			roleName,
			memberName,
		)

		if err != nil {
			log.Error(err, "unable to grant role membership")
			return err
		}
	} else if isMemberOf && !rolemembership.Spec.Granted {
		// Revoke role membership if currently a member but should not be granted
		err := pg.RevokeRole(
			handler,
			roleName,
			memberName,
		)
		if err != nil {
			log.Error(err, "unable to revoke role membership")
			return err
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgresRoleMembershipReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pgv1.PostgresRoleMembership{}).
		Named("postgresrolemembership").
		Complete(r)
}

func (r *PostgresRoleMembershipReconciler) setProgressing(ctx context.Context, rolemembership *pgv1.PostgresRoleMembership, message string) (bool, error) {
	changed := helpers.SetConditionStatus(&rolemembership.Status.Conditions, constants.ConditionProgressing, "Reconciling", message, rolemembership.Generation)
	if !changed {
		return changed, nil
	}
	rolemembership.Status.Phase = constants.ConditionProgressing
	return changed, r.Status().Update(ctx, rolemembership)
}

func (r *PostgresRoleMembershipReconciler) setReady(ctx context.Context, rolemembership *pgv1.PostgresRoleMembership, message string) (bool, error) {
	changed := helpers.SetConditionStatus(&rolemembership.Status.Conditions, constants.ConditionReady, "Reconciled", message, rolemembership.Generation)
	if !changed {
		return changed, nil
	}
	rolemembership.Status.Phase = constants.ConditionReady
	return changed, r.Status().Update(ctx, rolemembership)
}

func (r *PostgresRoleMembershipReconciler) setDegraded(ctx context.Context, rolemembership *pgv1.PostgresRoleMembership, message string) (bool, error) {
	changed := helpers.SetConditionStatus(&rolemembership.Status.Conditions, constants.ConditionDegraded, "ReconcileFailed", message, rolemembership.Generation)
	if !changed {
		return changed, nil
	}
	rolemembership.Status.Phase = constants.ConditionDegraded
	return changed, r.Status().Update(ctx, rolemembership)
}
