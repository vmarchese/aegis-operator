/*
Copyright 2024.

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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aegisv1 "github.com/vmarchese/aegis-operator/api/v1"
)

const (
	typeAvailableHashicorpVaultProvider = "Available"
	hashicorpVaultFinalizerName         = "idprovider.aegis.aegisproxy.io"
)

// HashicorpVaultProviderReconciler reconciles a HashicorpVaultProvider object
type HashicorpVaultProviderReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=hashicorpvaultproviders,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=hashicorpvaultproviders/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=hashicorpvaultproviders/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HashicorpVaultProvider object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *HashicorpVaultProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Log the request details
	log.Info("Reconciling HashicorpVaultProvider", "name", req.Name, "namespace", req.Namespace)
	// Fetch the Identity object
	vault := &aegisv1.HashicorpVaultProvider{}
	err := r.Get(ctx, req.NamespacedName, vault)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("HashicorpVaultProvider resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch HashicorpVaultProvider")
		return ctrl.Result{}, err
	}

	if !vault.ObjectMeta.DeletionTimestamp.IsZero() {
		log.Info("HashicorpVaultProvider is being deleted")
		// delete all identities
		// List all identities with matching provider label
		identityList := &aegisv1.IdentityList{}
		if err := r.List(ctx, identityList, client.MatchingLabels{
			"aegis.aegisproxy.io/identity.provider": vault.Name,
		}); err != nil {
			log.Error(err, "Failed to list identities")
			return ctrl.Result{}, err
		}

		// Delete each identity
		for _, identity := range identityList.Items {
			log.Info("Deleting identity", "identity", identity.Name)
			if err := r.Delete(ctx, &identity); err != nil {
				log.Error(err, "Failed to delete identity", "identity", identity.Name)
				return ctrl.Result{}, err
			}
		}
		// check for deletion
		for _, identity := range identityList.Items {
			idObj := &aegisv1.Identity{}
			if err := r.Get(ctx, types.NamespacedName{Name: identity.Name, Namespace: identity.Namespace}, idObj); err == nil {
				log.Error(err, "identity not deleted", "identity", identity.Name)
				return ctrl.Result{Requeue: true}, nil
			}
		}

		// now delete the vault provider finalizer
		if controllerutil.ContainsFinalizer(vault, hashicorpVaultFinalizerName) {
			log.Info("Deleting HashicorpVaultProvider finalizer")
			updated := controllerutil.RemoveFinalizer(vault, hashicorpVaultFinalizerName)
			if !updated {
				return ctrl.Result{}, err
			}
			if err := r.Update(ctx, vault); err != nil {
				log.Error(err, "Failed to update HashicorpVaultProvider to remove finalizer")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if len(vault.Status.Conditions) == 0 {
		meta.SetStatusCondition(&vault.Status.Conditions,
			metav1.Condition{Type: typeAvailableHashicorpVaultProvider,
				Status:  metav1.ConditionUnknown,
				Reason:  "Reconciling",
				Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, vault); err != nil {
			log.Error(err, "Failed to update HashicorpVaultProvider status to Reconciling")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, vault); err != nil {
			log.Error(err, "Failed to re-fetch HashicorpVaultProvider")
			return ctrl.Result{}, err
		}
	}

	// appending finalizer
	if !controllerutil.ContainsFinalizer(vault, hashicorpVaultFinalizerName) {
		updated := controllerutil.AddFinalizer(vault, hashicorpVaultFinalizerName)
		if !updated {
			log.Error(err, "Failed to update HashicorpVaultProvider with finalizer")
			return ctrl.Result{}, err
		}
		if err := r.Update(ctx, vault); err != nil {
			log.Error(err, "Failed to update HashicorpVaultProvider to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}
	meta.SetStatusCondition(&vault.Status.Conditions,
		metav1.Condition{Type: typeAvailableHashicorpVaultProvider,
			Status:  metav1.ConditionTrue,
			Reason:  "Reconciled",
			Message: "HashicorpVaultProvider reconciled"})
	if err := r.Status().Update(ctx, vault); err != nil {
		log.Error(err, "Failed to update HashicorpVaultProvider status to Reconciled")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HashicorpVaultProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aegisv1.HashicorpVaultProvider{}).
		Complete(r)
}
