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
	awsProviderFinalizerName = "idprovider.aegis.aegisproxy.io"
	typeAvailableAWSProvider = "Available"
)

// AWSProviderReconciler reconciles a AWSProvider object
type AWSProviderReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=awsproviders,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=awsproviders/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=awsproviders/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AWSProvider object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *AWSProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := log.FromContext(ctx)

	// Log the request details
	log.Info("Reconciling AWSProvider", "name", req.Name, "namespace", req.Namespace)
	// Fetch the Identity object
	awsIAM := &aegisv1.AWSProvider{}
	err := r.Get(ctx, req.NamespacedName, awsIAM)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("AWSProvider resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch AWSProvider")
		return ctrl.Result{}, err
	}

	if !awsIAM.ObjectMeta.DeletionTimestamp.IsZero() {
		log.Info("AWSProvider is being deleted")
		// delete all identities
		// List all identities with matching provider label
		identityList := &aegisv1.IdentityList{}
		if err := r.List(ctx, identityList, client.MatchingLabels{
			"aegis.aegisproxy.io/identity.provider": awsIAM.Name,
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
		if controllerutil.ContainsFinalizer(awsIAM, awsProviderFinalizerName) {
			log.Info("Deleting AWSProvider finalizer")
			updated := controllerutil.RemoveFinalizer(awsIAM, awsProviderFinalizerName)
			if !updated {
				return ctrl.Result{}, err
			}
			if err := r.Update(ctx, awsIAM); err != nil {
				log.Error(err, "Failed to update AWSProvider to remove finalizer")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if len(awsIAM.Status.Conditions) == 0 {
		meta.SetStatusCondition(&awsIAM.Status.Conditions,
			metav1.Condition{Type: typeAvailableAWSProvider,
				Status:  metav1.ConditionUnknown,
				Reason:  "Reconciling",
				Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, awsIAM); err != nil {
			log.Error(err, "Failed to update AWSProvider status to Reconciling")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, awsIAM); err != nil {
			log.Error(err, "Failed to re-fetch AWSProvider")
			return ctrl.Result{}, err
		}
	}

	// appending finalizer
	if !controllerutil.ContainsFinalizer(awsIAM, awsProviderFinalizerName) {
		updated := controllerutil.AddFinalizer(awsIAM, awsProviderFinalizerName)
		if !updated {
			log.Error(err, "Failed to update AWSProvider with finalizer")
			return ctrl.Result{}, err
		}
		if err := r.Update(ctx, awsIAM); err != nil {
			log.Error(err, "Failed to update AWSProvider to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}
	meta.SetStatusCondition(&awsIAM.Status.Conditions,
		metav1.Condition{Type: typeAvailableAWSProvider,
			Status:  metav1.ConditionTrue,
			Reason:  "Reconciled",
			Message: "AWSProvider reconciled"})
	if err := r.Status().Update(ctx, awsIAM); err != nil {
		log.Error(err, "Failed to update AWSProvider status to Reconciled")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AWSProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aegisv1.AWSProvider{}).
		Complete(r)
}
