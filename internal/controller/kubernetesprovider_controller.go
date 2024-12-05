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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
	kubernetesProviderFinalizerName = "idprovider.aegis.aegisproxy.io"
	typeAvailableKubernetesProvider = "Available"
)

// KubernetesProviderReconciler reconciles a KubernetesProvider object
type KubernetesProviderReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=kubernetesproviders,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=kubernetesproviders/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=kubernetesproviders/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the KubernetesProvider object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *KubernetesProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Log the request details
	log.Info("Reconciling KubernetesProvider", "name", req.Name, "namespace", req.Namespace)
	// Fetch the Identity object
	thisKube := &aegisv1.KubernetesProvider{}
	err := r.Get(ctx, req.NamespacedName, thisKube)
	if err != nil {

		if apierrors.IsNotFound(err) {
			log.Info("KubernetesProvider resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch KubernetesProvider")
		return ctrl.Result{}, err
	}

	if !thisKube.ObjectMeta.DeletionTimestamp.IsZero() {
		log.Info("KubernetesProvider is being deleted")
		// delete all identities
		// List all identities with matching provider label
		identityList := &aegisv1.IdentityList{}
		if err := r.List(ctx, identityList, client.MatchingLabels{
			"aegis.aegisproxy.io/identity.provider": thisKube.Name,
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
		if controllerutil.ContainsFinalizer(thisKube, kubernetesProviderFinalizerName) {
			log.Info("Deleting KubernetesProvider finalizer")
			updated := controllerutil.RemoveFinalizer(thisKube, kubernetesProviderFinalizerName)
			if !updated {
				return ctrl.Result{}, err
			}
			if err := r.Update(ctx, thisKube); err != nil {
				log.Error(err, "Failed to update KubernetesProvider to remove finalizer")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if len(thisKube.Status.Conditions) == 0 {
		meta.SetStatusCondition(&thisKube.Status.Conditions,
			metav1.Condition{Type: typeAvailableKubernetesProvider,
				Status:  metav1.ConditionUnknown,
				Reason:  "Reconciling",
				Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, thisKube); err != nil {
			log.Error(err, "Failed to update KubernetesProvider status to Reconciling")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, thisKube); err != nil {
			log.Error(err, "Failed to re-fetch KubernetesProvider")
			return ctrl.Result{}, err
		}
	}

	// appending finalizer
	if !controllerutil.ContainsFinalizer(thisKube, kubernetesProviderFinalizerName) {
		updated := controllerutil.AddFinalizer(thisKube, kubernetesProviderFinalizerName)
		if !updated {
			log.Error(err, "Failed to update KubernetesProvider with finalizer")
			return ctrl.Result{}, err
		}
		if err := r.Update(ctx, thisKube); err != nil {
			log.Error(err, "Failed to update KubernetesProvider to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	meta.SetStatusCondition(&thisKube.Status.Conditions,
		metav1.Condition{Type: typeAvailableKubernetesProvider,
			Status:  metav1.ConditionTrue,
			Reason:  "Reconciled",
			Message: "KubernetesProvider reconciled"})
	issuer, err := r.getIssuer()
	if err != nil {
		log.Error(err, "Failed to get issuer")
		return ctrl.Result{}, err
	}
	thisKube.Status.Issuer = issuer
	if err := r.Status().Update(ctx, thisKube); err != nil {
		log.Error(err, "Failed to update KubernetesProvider status to Reconciled")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
func (r *KubernetesProviderReconciler) getIssuer() (string, error) {
	token, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return "", err
	}

	// Split the token into its parts
	parts := strings.Split(string(token), ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid token format")
	}

	// Decode the payload part (second part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}

	// Define a struct to hold the payload data
	var claims struct {
		Issuer string `json:"iss"`
	}

	// Unmarshal the payload into the struct
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", err
	}

	return claims.Issuer, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubernetesProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aegisv1.KubernetesProvider{}).
		Complete(r)
}
