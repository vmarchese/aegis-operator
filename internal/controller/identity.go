package controller

import (
	"context"

	aegisv1 "github.com/vmarchese/aegis-operator/api/v1"
)

type IdentityHelper interface {
	CreateIdentity(ctx context.Context, identity *aegisv1.Identity) (map[string]string, error)
	GetIdentity(ctx context.Context, identity *aegisv1.Identity) (bool, error)
	DeleteIdentity(ctx context.Context, identity *aegisv1.Identity) error
	GetName() string
}
