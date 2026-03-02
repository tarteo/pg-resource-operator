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
	corev1 "k8s.io/api/core/v1"
)

// RoleReconciler reconciles a Role object
type RoleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pg.onestein.nl,resources=postgres,verbs=get;list;watch
// +kubebuilder:rbac:groups=pg.onestein.nl,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pg.onestein.nl,resources=roles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pg.onestein.nl,resources=roles/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Role object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *RoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("namespace", req.NamespacedName)
	log.Info("reconciling role")

	var role pgv1.Role
	if err := r.Get(ctx, req.NamespacedName, &role); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to find role (in kubernetes)")
		return ctrl.Result{}, err
	}

	// Apply finalizer
	if role.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&role, constants.Finalizer) {
			controllerutil.AddFinalizer(&role, constants.Finalizer)
			if err := r.Update(ctx, &role); err != nil {
				log.Error(err, "unable to update role with finalizer")
				return ctrl.Result{}, err
			}

			// Return and requeue to get fresh object
			return ctrl.Result{Requeue: true}, nil
		}
		// Set progressing status
		if changed, err := r.setProgressing(ctx, &role, "Reconciling role"); err != nil {
			log.Error(err, "unable to set progressing status")
			return ctrl.Result{}, err
		} else if changed {
			// Requeue to get fresh object with updated status
			return ctrl.Result{Requeue: true}, nil
		}
	}

	if err := r.ReconcileResource(ctx, &role); err != nil {
		log.Error(err, "unable to reconcile role")

		// Set degraded status
		if _, errStatus := r.setDegraded(ctx, &role, err.Error()); errStatus != nil {
			log.Error(errStatus, "unable to set degraded status")
			return ctrl.Result{}, errStatus
		}
		return ctrl.Result{}, err
	}

	// Remove finalizer if deletion timestamp is set
	if !role.ObjectMeta.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(&role, constants.Finalizer) {
		controllerutil.RemoveFinalizer(&role, constants.Finalizer)
		if err := r.Update(ctx, &role); err != nil {
			log.Error(err, "unable to update role with finalizer")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Set ready status
	if _, err := r.setReady(ctx, &role, "Role successfully reconciled"); err != nil {
		log.Error(err, "unable to set ready status")
		return ctrl.Result{}, err
	}

	log.Info("successfully reconciled role")

	return ctrl.Result{}, nil
}

func (r *RoleReconciler) ReconcileResource(ctx context.Context, role *pgv1.Role) error {
	log := log.FromContext(ctx).WithValues("namespace", types.NamespacedName{Namespace: role.Namespace, Name: role.Name})

	handler, err := role.Spec.PostgresRef.GetPostgresHandle(ctx, r.Client, role.Namespace)
	if err != nil {
		return err
	}
	defer handler.Close()

	// Check if role exists
	exists, err := pg.RoleExists(handler, role.Spec.Name)
	if err != nil {
		return err
	}

	if !role.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion
		if exists {
			err := pg.DropRole(handler, role.Spec.Name)
			if err != nil {
				log.Error(err, "unable to drop role")
				return err
			}
		}
		return nil
	}

	// get password if specified
	password, err := r.GetPassword(ctx, role)
	if err != nil {
		log.Error(err, "unable to get role password")
		return err
	}

	if !exists {
		// Create role
		err := pg.CreateRole(
			handler,
			role.Spec.Name, password,
		)

		if err != nil {
			log.Error(err, "unable to create role")
			return err
		}
	} else {
		// Update role password this will set the to null if no password secret is specified, which will remove the password from the role
		err := pg.AlterRolePassword(
			handler,
			role.Spec.Name, password,
		)
		if err != nil {
			log.Error(err, "unable to change role password")
			return err
		}
	}

	return nil
}

func (r *RoleReconciler) GetPassword(ctx context.Context, role *pgv1.Role) (*string, error) {
	// If no password secret is specified, return nil to indicate that the password should not be set or updated
	if role.Spec.PasswordSecret == nil {
		return nil, nil
	}

	namespace := role.Spec.PasswordSecret.Namespace
	if namespace == "" {
		namespace = role.Namespace
	}

	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      role.Spec.PasswordSecret.Name,
	}, &secret); err != nil {
		return nil, err
	}
	passwordBytes, exists := secret.Data[role.Spec.PasswordKey]
	if !exists {
		return nil, fmt.Errorf("password key (%s) does not exist in secret", role.Spec.PasswordKey)
	}
	password := string(passwordBytes)
	return &password, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pgv1.Role{}).
		Named("role").
		Complete(r)
}

func (r *RoleReconciler) setProgressing(ctx context.Context, role *pgv1.Role, message string) (bool, error) {
	changed := helpers.SetConditionStatus(&role.Status.Conditions, constants.ConditionProgressing, "Reconciling", message, role.Generation)
	if !changed {
		return changed, nil
	}
	role.Status.Phase = constants.ConditionProgressing
	return changed, r.Status().Update(ctx, role)
}

func (r *RoleReconciler) setReady(ctx context.Context, role *pgv1.Role, message string) (bool, error) {
	changed := helpers.SetConditionStatus(&role.Status.Conditions, constants.ConditionReady, "Reconciled", message, role.Generation)
	if !changed {
		return changed, nil
	}
	role.Status.Phase = constants.ConditionReady
	return changed, r.Status().Update(ctx, role)
}

func (r *RoleReconciler) setDegraded(ctx context.Context, role *pgv1.Role, message string) (bool, error) {
	changed := helpers.SetConditionStatus(&role.Status.Conditions, constants.ConditionDegraded, "ReconcileFailed", message, role.Generation)
	if !changed {
		return changed, nil
	}
	role.Status.Phase = constants.ConditionDegraded
	return changed, r.Status().Update(ctx, role)
}
