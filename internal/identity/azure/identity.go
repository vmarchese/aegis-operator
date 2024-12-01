package azure

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	azureauth "github.com/microsoft/kiota-authentication-azure-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	aegisv1 "github.com/vmarchese/aegis-operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ProviderName = "azure"
	K8STokenPath = "/var/run/secrets/tokens/azure_token"

	StatusMetaAegisIdentityID    = "aegis.identity.id"
	StatusMetaAegisAzureTenantID = "aegis.identity.azure.tenantid"
	StatusMetaAegisProvider      = "aegis.identity.provider"
)

type IdentityHelper struct {
	tenantID string
	clientID string
	retries  int
	timeout  time.Duration
}

func New(tenantID string, clientID string) *IdentityHelper {
	return &IdentityHelper{
		tenantID: tenantID,
		clientID: clientID,
		retries:  3,
		timeout:  10 * time.Second,
	}
}

func (h *IdentityHelper) GetName() string {
	return ProviderName
}

func (h *IdentityHelper) getGraphClient(ctx context.Context) (*msgraphsdk.GraphServiceClient, error) {
	log := log.FromContext(ctx)

	cred, err := azidentity.NewClientAssertionCredential(h.tenantID, h.clientID, h.GetToken, &azidentity.ClientAssertionCredentialOptions{
		ClientOptions: azcore.ClientOptions{
			Retry: policy.RetryOptions{
				MaxRetries: int32(h.retries),
				TryTimeout: h.timeout,
			},
		},
	})
	if err != nil {
		log.Error(err, "Failed to create Azure Identity credential")
		return nil, err
	}

	authProvider, err := azureauth.NewAzureIdentityAuthenticationProviderWithScopes(cred, []string{"https://graph.microsoft.com/.default"})
	if err != nil {
		log.Error(err, "Failed to create Azure AuthenticationProvider")
		return nil, err
	}

	adapter, err := msgraphsdk.NewGraphRequestAdapter(authProvider)
	if err != nil {
		log.Error(err, "Failed to create Azure GraphRequestAdapter")
		return nil, err
	}

	log.Info("Creating Azure GraphServiceClient")
	client := msgraphsdk.NewGraphServiceClient(adapter)
	return client, nil
}

func (h *IdentityHelper) CreateIdentity(ctx context.Context, identity *aegisv1.Identity) (map[string]string, error) {
	log := log.FromContext(ctx)

	client, err := h.getGraphClient(ctx)
	if err != nil {
		return nil, err
	}

	// Check if application with the same name already exists
	log.Info("Checking if application already exists")
	apps, err := client.Applications().Get(ctx, nil)
	if err != nil {
		log.Error(err, "Failed to list applications")
		return nil, err
	}

	for _, app := range apps.GetValue() {
		if *app.GetDisplayName() == identity.Name {
			log.Info("Application already exists", "applicationID", *app.GetId())
			return map[string]string{
				StatusMetaAegisIdentityID:    *app.GetId(),
				StatusMetaAegisAzureTenantID: h.tenantID,
				StatusMetaAegisProvider:      ProviderName,
			}, nil
		}
	}

	app := models.NewApplication()
	app.SetDisplayName(&identity.Name)

	log.Info("Creating Azure Application")
	createdApp, err := client.Applications().Post(ctx, app, nil)
	if err != nil {
		log.Error(err, "Failed to create Azure Application")
		return nil, err
	}
	applicationID := *createdApp.GetId()

	log.Info("Getting issuer")
	issuer, err := h.GetIssuer(ctx)
	if err != nil {
		log.Error(err, "Failed to get issuer")
		return nil, err
	}

	log.Info("Creating FederatedIdentityCredential")
	subject := fmt.Sprintf("system:serviceaccount:%s:%s", identity.Namespace, identity.Name)
	federatedIdentity := models.NewFederatedIdentityCredential()
	federatedIdentity.SetName(&identity.Name)
	federatedIdentity.SetIssuer(&issuer)
	federatedIdentity.SetSubject(&subject)
	federatedIdentity.SetAudiences([]string{"api://AzureADTokenExchange"})

	fiRequestBody := federatedIdentity

	_, err = client.
		Applications().ByApplicationId(applicationID).
		FederatedIdentityCredentials().
		Post(ctx, fiRequestBody, nil)
	if err != nil {
		log.Error(err, "Failed to create FederatedIdentityCredential")
		return nil, err
	}
	return map[string]string{
		StatusMetaAegisIdentityID:    applicationID,
		StatusMetaAegisAzureTenantID: h.tenantID,
		StatusMetaAegisProvider:      ProviderName,
	}, nil
}

func (h *IdentityHelper) GetIdentity(ctx context.Context, identity *aegisv1.Identity) (bool, error) {
	return false, fmt.Errorf("not implemented")
}

func (h *IdentityHelper) DeleteIdentity(ctx context.Context, identity *aegisv1.Identity) error {
	log := log.FromContext(ctx)

	client, err := h.getGraphClient(ctx)
	if err != nil {
		return err
	}

	// Find the application by name
	log.Info("Finding application to delete")
	apps, err := client.Applications().Get(ctx, nil)
	if err != nil {
		log.Error(err, "Failed to list applications")
		return err
	}

	var applicationID string
	for _, app := range apps.GetValue() {
		if *app.GetDisplayName() == identity.Name {
			applicationID = *app.GetId()
			break
		}
	}

	if applicationID == "" {
		log.Info("Application not found", "name", identity.Name)
		return fmt.Errorf("application not found: %s", identity.Name)
	}

	// Delete the application
	log.Info("Deleting application", "applicationID", applicationID)
	err = client.Applications().ByApplicationId(applicationID).Delete(ctx, nil)
	if err != nil {
		log.Error(err, "Failed to delete application", "applicationID", applicationID)
		return err
	}

	log.Info("Application deleted successfully", "applicationID", applicationID)
	return nil
}

func (h *IdentityHelper) GetToken(ctx context.Context) (string, error) {
	token, err := os.ReadFile(K8STokenPath)
	if err != nil {
		return "", err
	}
	return string(token), nil
}

func (h *IdentityHelper) GetIssuer(ctx context.Context) (string, error) {
	token, err := os.ReadFile(K8STokenPath)
	if err != nil {
		return "", err
	}
	parts := strings.Split(string(token), ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid token")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", err
	}

	issuer, ok := claims["iss"].(string)
	if !ok {
		return "", fmt.Errorf("issuer not found in token")
	}

	return issuer, nil
}
