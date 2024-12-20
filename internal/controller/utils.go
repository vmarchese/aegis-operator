package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func ensureResourceExists(ctx context.Context, c client.Client, obj client.Object) error {
	log := log.FromContext(ctx)

	// Attempt to get the object
	err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	if err == nil {
		// Object already exists, no action needed
		log.Info("Resource already exists", "resource", obj.GetName())
		return nil
	}

	// Check if the error is a "NotFound" error
	if err := client.IgnoreNotFound(err); err != nil {
		// An unexpected error occurred, return it
		log.Error(err, "Failed to check for existing resource")
		return err
	}

	// Object does not exist, attempt to create it
	err = c.Create(ctx, obj)
	if err != nil {
		log.Error(err, "Failed to create resource", "resource", obj.GetName())
		return err
	}

	log.Info("Successfully created resource", "resource", obj.GetName())
	return nil
}

func ensureResourceExistsWithControllerReference(ctx context.Context, c client.Client, obj client.Object, controller client.Object, scheme *runtime.Scheme) error {
	log := log.FromContext(ctx)

	log.Info("Ensuring resource exists", "resource", obj.GetName(), "controller", controller.GetName())

	// Attempt to get the object
	err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	if err == nil {
		// Object already exists, no action needed
		log.Info("Resource already exists", "resource", obj.GetName())
		return nil
	}

	// Check if the error is a "NotFound" error
	if err := client.IgnoreNotFound(err); err != nil {
		// An unexpected error occurred, return it
		log.Error(err, "Failed to check for existing resource")
		return err
	}

	err = ctrl.SetControllerReference(controller, obj, scheme)
	if err != nil {
		log.Error(err, "Failed to set controller reference")
		return err
	}

	// Object does not exist, attempt to create it
	err = c.Create(ctx, obj)
	if err != nil {
		log.Error(err, "Failed to create resource", "resource", obj.GetName())
		return err
	}

	log.Info("Successfully created resource", "resource", obj.GetName())
	return nil
}

func ensureAegisProxyServiceAccount(ctx context.Context, c client.Client, namespace string) error {
	log := log.FromContext(ctx)
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      aegisProxyIdentity,
			Namespace: namespace,
		},
	}
	err := c.Get(ctx, client.ObjectKeyFromObject(serviceAccount), serviceAccount)
	if err == nil {
		return nil
	}
	if err := client.IgnoreNotFound(err); err != nil {
		// An unexpected error occurred, return it
		log.Error(err, "Failed to check for existing resource")
		return err
	}

	// Object does not exist, attempt to create it
	err = c.Create(ctx, serviceAccount)
	if err != nil {
		log.Error(err, "Failed to create resource", "resource", serviceAccount.GetName())
		return err
	}

	log.Info("Successfully created resource", "resource", serviceAccount.GetName())
	return nil

}
