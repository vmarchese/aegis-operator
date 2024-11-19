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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aegisv1 "github.com/vmarchese/aegis-operator/api/v1"
	"github.com/vmarchese/aegis-operator/internal/identity/hashicorpvault"
)

const (
	typeAvailableIdentity = "Available"
	finalizerName         = "identity.aegis.aegisproxy.io"
)

// IdentityReconciler reconciles a Identity object
type IdentityReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=identities,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=identities/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=identities/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;create;update;delete;list;watch;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Identity object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *IdentityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Log the request details
	log.Info("Reconciling Identity", "name", req.Name, "namespace", req.Namespace, "request", req)
	// Fetch the Identity object
	identity := &aegisv1.Identity{}
	if err := r.Get(ctx, req.NamespacedName, identity); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("identity resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Identity")
		return ctrl.Result{}, err
	}

	idProvider, err := r.findProvider(ctx, req, identity.Spec.Provider)
	if err != nil {
		log.Error(err, "Failed to find provider")
		return ctrl.Result{}, err
	}

	// check deletion timestamp
	if !identity.ObjectMeta.DeletionTimestamp.IsZero() {
		log.Info("identity is being deleted")
		err = idProvider.DeleteIdentity(ctx, identity)
		if err != nil {
			log.Error(err, "Failed to delete identity on vault")
			return ctrl.Result{}, err
		}

		if containsString(identity.ObjectMeta.Finalizers, finalizerName) {
			identity.ObjectMeta.Finalizers = removeString(identity.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(ctx, identity); err != nil {
				return ctrl.Result{}, err
			}
		}

		// delete service account
		err = r.Delete(ctx, &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      identity.Name,
				Namespace: identity.Namespace,
			},
		})
		if err != nil {
			log.Error(err, "Failed to delete service account")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// set status to reconciling if not set
	if len(identity.Status.Conditions) == 0 {
		meta.SetStatusCondition(&identity.Status.Conditions, metav1.Condition{Type: typeAvailableIdentity, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, identity); err != nil {
			log.Error(err, "Failed to update Identity status")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, identity); err != nil {
			log.Error(err, "Failed to re-fetch identity")
			return ctrl.Result{}, err
		}
	}

	// appending finalizer
	if !containsString(identity.ObjectMeta.Finalizers, finalizerName) {
		identity.ObjectMeta.Finalizers = append(identity.ObjectMeta.Finalizers, finalizerName)
		if err := r.Update(ctx, identity); err != nil {
			log.Error(err, "Failed to update Identity with finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	meta.SetStatusCondition(&identity.Status.Conditions,
		metav1.Condition{Type: typeAvailableIdentity, Status: metav1.ConditionTrue, Reason: "Reconciled", Message: "Identity reconciled"})
	identity.Status.Provider = idProvider.GetName()
	if err := r.Status().Update(ctx, identity); err != nil {
		log.Error(err, "Failed to update Identity status")
		return ctrl.Result{}, err
	}

	idmeta, err := idProvider.CreateIdentity(ctx, identity)
	if err != nil {
		log.Error(err, "Failed to create identity on vault")
		return ctrl.Result{}, err
	}

	identity.Status.Metadata = idmeta
	if err := r.Status().Update(ctx, identity); err != nil {
		log.Error(err, "Failed to update Identity status")
		return ctrl.Result{}, err
	}

	// creating service account if not exists
	serviceAccount := &corev1.ServiceAccount{}
	err = r.Get(ctx, client.ObjectKey{Name: identity.Name, Namespace: identity.Namespace}, serviceAccount)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get service account")
			serviceAccount = &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      identity.Name,
					Namespace: identity.Namespace,
				},
			}
			if err := r.Create(ctx, serviceAccount); err != nil {
				log.Error(err, "Failed to create service account")
				return ctrl.Result{}, err
			}
		} else {
			log.Error(err, "Failed to get service account")
			return ctrl.Result{}, err
		}
	}

	log.Info("Reconciled IdentitySpec", "spec", identity.Spec)
	return ctrl.Result{}, nil
}

func (r *IdentityReconciler) findProvider(ctx context.Context, req ctrl.Request, providerName string) (IdentityHelper, error) {

	provider := &aegisv1.HashicorpVaultProvider{}
	err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: providerName}, provider)
	if err != nil {
		return nil, err
	}
	return hashicorpvault.New(provider.Spec.VaultAddress), nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *IdentityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aegisv1.Identity{}).
		Complete(r)
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
func removeString(slice []string, s string) []string {
	var result []string
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}
