package azure

import (
	"context"

	aegisv1 "github.com/vmarchese/aegis-operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func GetIdentity(ctx context.Context, identity *aegisv1.Identity) (bool, error) {
	log := log.FromContext(ctx)
	log.Info("Getting identity on Azure", "identity", identity)
	return false, nil

}

func CreateIdentity(ctx context.Context, identity *aegisv1.Identity) (aegisv1.IdentityStatus, error) {
	log := log.FromContext(ctx)
	log.Info("Creating identity on Azure", "identity", identity)

	return aegisv1.IdentityStatus{
		ApplicationID: "app-1234567890",
		ObjectID:      "obj-1234567890",
	}, nil

}
