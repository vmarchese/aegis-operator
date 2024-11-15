package hashicorpvault

import (
	"context"
	"fmt"
	"os"

	vault_client "github.com/hashicorp/vault-client-go"
	vault_client_schema "github.com/hashicorp/vault-client-go/schema"
	aegisv1 "github.com/vmarchese/aegis-operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type IdentityHelper struct {
	vaultAddress string
}

func New(vaultAddress string) *IdentityHelper {
	return &IdentityHelper{vaultAddress: vaultAddress}
}

func (h *IdentityHelper) CreateIdentity(ctx context.Context, identity *aegisv1.Identity) (map[string]string, error) {
	client, err := h.getClient(ctx)
	if err != nil {
		return nil, err
	}

	return h.createIdentity(ctx, client, identity)

}

func (h *IdentityHelper) GetIdentity(ctx context.Context, identity *aegisv1.Identity) (bool, error) {
	return false, nil
}
func (h *IdentityHelper) DeleteIdentity(ctx context.Context, identity *aegisv1.Identity) error {
	log := log.FromContext(ctx)
	saName := fmt.Sprintf("system:serviceaccount:%s:%s", identity.Namespace, identity.Name)

	client, err := h.getClient(ctx)
	if err != nil {
		return err
	}

	//delete jwt role
	resp, err := client.Auth.JwtDeleteRole(ctx, identity.Name)
	if err != nil {
		return err
	}
	log.Info("deleted jwt role", "response", resp)

	// delete oidc role
	resp, err = client.Identity.OidcDeleteRole(ctx, identity.Name)
	if err != nil {
		return err
	}
	log.Info("deleted oidc role", "response", resp)

	// delete entity alias
	resp, err = client.Identity.EntityReadByName(ctx, saName)
	if err != nil {
		return err
	}

	aliases := resp.Data["aliases"].([]interface{})
	for _, alias := range aliases {
		resp, err = client.Identity.EntityDeleteAliasById(ctx, alias.(map[string]interface{})["id"].(string))
		if err != nil {
			return err
		}
		log.Info("deleted alias", "response", resp)
	}

	log.Info("deleted entity aliases")

	//delete entity
	_, err = client.Identity.EntityDeleteByName(ctx, saName)
	if err != nil {
		return err
	}

	return nil
}

func (h *IdentityHelper) createIdentity(ctx context.Context, client *vault_client.Client, identity *aegisv1.Identity) (map[string]string, error) {
	log := log.FromContext(ctx)

	saName := fmt.Sprintf("system:serviceaccount:%s:%s", identity.Namespace, identity.Name)

	resp, err := client.Identity.EntityCreate(ctx, vault_client_schema.EntityCreateRequest{
		Name: saName,
		Metadata: map[string]interface{}{
			"platform":  "aegis",
			"identity":  identity.Name,
			"namespace": identity.Namespace,
		},
	})
	if err != nil {
		log.Error(err, "unable to create identity")
		return nil, err
	}
	log.Info("created identity", "response", resp)
	resp, err = client.Identity.EntityReadByName(ctx, saName)
	if err != nil {
		return nil, err
	}
	log.Info("got identity", "response", resp, "id", resp.Data["id"])
	e := resp.Data["id"]
	entityId := e.(string)

	resp, err = client.Auth.JwtWriteRole(ctx, identity.Name, vault_client_schema.JwtWriteRoleRequest{
		RoleType:       "jwt",
		UserClaim:      "sub",
		BoundSubject:   saName,
		Policies:       []string{"default", "jwt_issuer"},
		BoundAudiences: []string{"vault"},
	})
	if err != nil {
		log.Error(err, "unable to create jwt role")
		return nil, err
	}
	log.Info("created jwt role", "response", resp)

	//get jwt accessor
	resp, err = client.System.AuthListEnabledMethods(ctx)
	if err != nil {
		return nil, err
	}
	accessor := resp.Data["jwt/"].(map[string]interface{})["accessor"].(string)

	// create entity alias
	resp, err = client.Identity.EntityCreateAlias(ctx, vault_client_schema.EntityCreateAliasRequest{
		Name:          saName,
		CanonicalId:   entityId,
		MountAccessor: accessor,
	})
	if err != nil {
		return nil, err
	}
	log.Info("created alias", "response", resp)

	resp, err = client.Identity.OidcWriteRole(ctx, identity.Name, vault_client_schema.OidcWriteRoleRequest{
		Key:      "aegis-key",
		Template: "ewogICAibmFtZSI6IHt7aWRlbnRpdHkuZW50aXR5Lm5hbWV9fSwKICAgInBsYXRmb3JtIjoge3tpZGVudGl0eS5lbnRpdHkubWV0YWRhdGEucGxhdGZvcm19fSwKICAgIm5iZiI6IHt7dGltZS5ub3d9fQp9",
		Ttl:      "1h",
	})
	if err != nil {
		return nil, err
	}
	log.Info("created oidc role", "response", resp)

	return map[string]string{
		"aegis.identity.id":            entityId,
		"aegis.identity.provider":      "hashicorp.vault",
		"aegis.identity.vault.address": h.vaultAddress,
	}, nil

}

func (h *IdentityHelper) getClient(ctx context.Context) (*vault_client.Client, error) {
	var err error
	log := log.FromContext(ctx)
	client, err := vault_client.New(
		vault_client.WithAddress(h.vaultAddress),
	)
	if err != nil {
		return nil, err
	}

	token, err := os.ReadFile("/var/run/secrets/tokens/token")
	if err != nil {
		return nil, err
	}
	log.Info("vault token", "token", string(token))

	authInfo, err := client.Auth.JwtLogin(ctx, vault_client_schema.JwtLoginRequest{
		Jwt:  string(token),
		Role: "aegis",
	})
	if err != nil {
		log.Error(err, "unable to log in with Kubernetes auth")
		return nil, err
	}

	vaultToken := authInfo.Auth.ClientToken
	log.Info("vault token", "token", vaultToken)
	if err := client.SetToken(vaultToken); err != nil {
		log.Error(err, "unable to set token")
		return nil, err
	}
	return client, nil
}
