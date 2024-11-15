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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aegisv1 "github.com/vmarchese/aegis-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
)

// IngressReconciler reconciles a Ingress object
type IngressReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=ingresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=aegis.aegisproxy.io,resources=ingresses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ingress object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Ingress instance
	ingress := &aegisv1.Ingress{}
	err := r.Get(ctx, req.NamespacedName, ingress)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Ingress resource not found. Ignoring since it must be deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// List all pods in the same namespace
	podList := &corev1.PodList{}
	listOpts := &client.ListOptions{
		Namespace: req.Namespace,
	}
	err = r.List(ctx, podList, listOpts)
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, pod := range podList.Items {
		sidecarName := fmt.Sprintf("%s-%s", IngressSidecarNamePrefix, pod.Name)
		// Check if the pod has the specific annotation
		mustInject := false
		for annotationKey, annotationValue := range pod.Annotations {
			if annotationKey == AnnotationIngress && annotationValue != "" {
				// Split the comma separated value into a slice of strings
				mustInject = true
				allowedList := strings.Split(annotationValue, ",")

				for _, identityName := range allowedList {
					identity := &aegisv1.Identity{}
					err = r.Get(ctx, client.ObjectKey{Name: identityName, Namespace: req.Namespace}, identity)
					if err != nil {
						if errors.IsNotFound(err) {
							log.Info("Identity resource not found. Ignoring since it must be deleted", "identity", identityName)
							continue
						}
						return ctrl.Result{}, err
					}
					log.Info("Found existing Identity object", "identity", identityName)
				}
			} else if strings.HasPrefix(annotationKey, AnnotationIngressClaim) {
				mustInject = true
				log.Info("Found Ingress claim annotation", "annotation", annotationKey)
			}
		}

		if !mustInject {
			continue
		}

		// Check if the sidecar is already injected
		sidecarInjected := false
		for _, container := range pod.Spec.Containers {
			if container.Name == sidecarName {
				sidecarInjected = true
				break
			}
		}

		// If the sidecar is not injected, add it
		if !sidecarInjected {
			pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
				Name:  sidecarName,
				Image: ingress.Spec.ProxyImage,
				// Add other necessary configurations for the sidecar
			})

			// Update the pod
			err = r.Update(ctx, &pod)
			if err != nil {
				log.Error(err, "Failed to update pod with sidecar", "pod", pod.Name)
				return ctrl.Result{}, err
			}
			log.Info("Injected Ingress sidecar into pod", "pod", pod.Name)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aegisv1.Ingress{}).
		Complete(r)
}
