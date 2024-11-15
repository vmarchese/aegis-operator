package azure

import (
	"context"
	"fmt"

	aegisv1 "github.com/vmarchese/aegis-operator/api/v1"
)

const ProviderName = "azure"

type IdentityHelper struct {
}

func (h *IdentityHelper) GetName() string {
	return ProviderName
}

func (h *IdentityHelper) CreateIdentity(ctx context.Context, identity *aegisv1.Identity) (map[string]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *IdentityHelper) GetIdentity(ctx context.Context, identity *aegisv1.Identity) (bool, error) {
	return false, fmt.Errorf("not implemented")
}

func (h *IdentityHelper) DeleteIdentity(ctx context.Context, identity *aegisv1.Identity) error {
	return fmt.Errorf("not implemented")
}
