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

// DatabaseReconciler reconciles a Database object
type DatabaseReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pg.onestein.nl,resources=postgres,verbs=get;list;watch
// +kubebuilder:rbac:groups=pg.onestein.nl,resources=databases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pg.onestein.nl,resources=databases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pg.onestein.nl,resources=databases/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Database object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("namespace", req.NamespacedName)
	log.Info("reconciling database")

	var database pgv1.Database
	if err := r.Get(ctx, req.NamespacedName, &database); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to find database (in kubernetes)")
		return ctrl.Result{}, err
	}

	// Apply finalizer
	if database.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&database, constants.Finalizer) {
			controllerutil.AddFinalizer(&database, constants.Finalizer)
			if err := r.Update(ctx, &database); err != nil {
				log.Error(err, "unable to update database with finalizer")
				return ctrl.Result{}, err
			}

			// Return and requeue to get fresh object
			return ctrl.Result{Requeue: true}, nil
		}
		// Set progressing status
		if changed, err := r.setProgressing(ctx, &database, "Reconciling database"); err != nil {
			log.Error(err, "unable to set progressing status")
			return ctrl.Result{}, err
		} else if changed {
			// Requeue to get fresh object with updated status
			return ctrl.Result{Requeue: true}, nil
		}
	}

	if err := r.ReconcileResource(ctx, &database); err != nil {
		log.Error(err, "unable to reconcile database")

		// Set degraded status
		if _, errStatus := r.setDegraded(ctx, &database, err.Error()); errStatus != nil {
			log.Error(errStatus, "unable to set degraded status")
			return ctrl.Result{}, errStatus
		}
		return ctrl.Result{}, err
	}

	// Remove finalizer if deletion timestamp is set
	if !database.ObjectMeta.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(&database, constants.Finalizer) {
		controllerutil.RemoveFinalizer(&database, constants.Finalizer)
		if err := r.Update(ctx, &database); err != nil {
			log.Error(err, "unable to update database with finalizer")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Set ready status
	if _, err := r.setReady(ctx, &database, "Database successfully reconciled"); err != nil {
		log.Error(err, "unable to set ready status")
		return ctrl.Result{}, err
	}

	log.Info("successfully reconciled database")

	return ctrl.Result{}, nil
}

func (r *DatabaseReconciler) ReconcileResource(ctx context.Context, database *pgv1.Database) error {
	log := log.FromContext(ctx).WithValues("namespace", types.NamespacedName{Namespace: database.Namespace, Name: database.Name})

	handler, err := database.Spec.PostgresRef.GetPostgresHandle(ctx, r.Client, database.Namespace)
	if err != nil {
		return err
	}
	defer handler.Close()

	// Check if database exists
	exists, err := pg.DatabaseExists(handler, database.Spec.Name)
	if err != nil {
		return err
	}

	if !database.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion
		if exists {
			err := pg.DropDatabase(handler, database.Spec.Name, true)
			if err != nil {
				log.Error(err, "unable to drop database")
				return err
			}
		}
		return nil
	}

	if !exists {
		// Create database
		err := pg.CreateDatabase(
			handler,
			database.Spec.Name, database.Spec.Owner, database.Spec.Template, database.Spec.Encoding,
		)

		if err != nil {
			log.Error(err, "unable to create database")
			return err
		}
	} else {
		// Update database owner
		err := pg.ChownDatabase(handler, database.Spec.Name, database.Spec.Owner)
		if err != nil {
			log.Error(err, "unable to change database owner")
			return err
		}
	}

	// Grant / revoke privileges
	for _, privilege := range database.Spec.Privileges {
		var granted []pg.Privilege
		var revoked []pg.Privilege
		if privilege.Connect {
			granted = append(granted, pg.CONNECT)
		} else {
			revoked = append(revoked, pg.CONNECT)
		}
		if privilege.Create {
			granted = append(granted, pg.CREATE)
		} else {
			revoked = append(revoked, pg.CREATE)
		}
		if privilege.Temporary {
			granted = append(granted, pg.TEMPORARY)
		} else {
			revoked = append(revoked, pg.TEMPORARY)
		}

		// Grant privileges
		if err := pg.GrantPrivileges(handler, database.Spec.Name, granted, privilege.Role); err != nil {
			log.Error(err, "unable to grant privileges")
			return err
		}

		// Revoke privileges
		if err := pg.RevokePrivileges(handler, database.Spec.Name, revoked, privilege.Role); err != nil {
			log.Error(err, "unable to revoke privileges")
			return err
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pgv1.Database{}).
		Named("database").
		Complete(r)
}

func (r *DatabaseReconciler) setProgressing(ctx context.Context, database *pgv1.Database, message string) (bool, error) {
	changed := helpers.SetConditionStatus(&database.Status.Conditions, constants.ConditionProgressing, "Reconciling", message, database.Generation)
	if !changed {
		return changed, nil
	}
	database.Status.Phase = constants.ConditionProgressing
	return changed, r.Status().Update(ctx, database)
}

func (r *DatabaseReconciler) setReady(ctx context.Context, database *pgv1.Database, message string) (bool, error) {
	changed := helpers.SetConditionStatus(&database.Status.Conditions, constants.ConditionReady, "Reconciled", message, database.Generation)
	if !changed {
		return changed, nil
	}
	database.Status.Phase = constants.ConditionReady
	return changed, r.Status().Update(ctx, database)
}

func (r *DatabaseReconciler) setDegraded(ctx context.Context, database *pgv1.Database, message string) (bool, error) {
	changed := helpers.SetConditionStatus(&database.Status.Conditions, constants.ConditionDegraded, "ReconcileFailed", message, database.Generation)
	if !changed {
		return changed, nil
	}
	database.Status.Phase = constants.ConditionDegraded
	return changed, r.Status().Update(ctx, database)
}
