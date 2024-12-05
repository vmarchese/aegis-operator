package kubernetes

import (
	"context"

	aegisv1 "github.com/vmarchese/aegis-operator/api/v1"
)

const ProviderName = "kubernetes"

type IdentityHelper struct {
}

func New() *IdentityHelper {
	return &IdentityHelper{}
}

func (h *IdentityHelper) GetName() string {
	return ProviderName
}

func (h *IdentityHelper) CreateIdentity(ctx context.Context, identity *aegisv1.Identity) (map[string]string, error) {
	return map[string]string{}, nil
}

func (h *IdentityHelper) GetIdentity(ctx context.Context, identity *aegisv1.Identity) (bool, error) {
	return true, nil
}

func (h *IdentityHelper) DeleteIdentity(ctx context.Context, identity *aegisv1.Identity) error {
	return nil
}
