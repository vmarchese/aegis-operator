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
	"github.com/google/uuid"
	azureauth "github.com/microsoft/kiota-authentication-azure-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/applications"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/policies"
	"github.com/microsoftgraph/msgraph-sdk-go/serviceprincipals"
	aegisv1 "github.com/vmarchese/aegis-operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ProviderName = "azure"
	K8STokenPath = "/var/run/secrets/tokens/azure_token"

	StatusMetaAegisIdentityObjectID = "aegis.identity.objectid"
	StatusMetaAegisIdentityID       = "aegis.identity.id"
	StatusMetaAegisAzureTenantID    = "aegis.identity.azure.tenantid"
	StatusMetaAegisProvider         = "aegis.identity.provider"

	aegisClaimsMappingPolicyName = "aegis_id"
)

type IdentityHelper struct {
	tenantID string
	clientID string
	retries  int
	timeout  time.Duration

	objectID           string
	servicePrincipalID string
	subject            string
	roleID             *uuid.UUID
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

	log.Info("Getting issuer")
	issuer, err := h.GetIssuer(ctx)
	if err != nil {
		log.Error(err, "Failed to get issuer")
		return nil, err
	}
	h.subject = fmt.Sprintf("system:serviceaccount:%s:%s", identity.Namespace, identity.Name)

	// Check and create application registration
	err = h.ensureAppRegistration(ctx, client, identity, issuer)
	if err != nil {
		return nil, err
	}

	// Create directory extension
	/*
		exName := "federatedSubject"
		extensionPropName := "extension_" + strings.Replace(h.clientID, "-", "", -1) + "_federatedSubject"

			err = h.createExtensionProperty(ctx, client, exName, extensionPropName)
			if err != nil {
				return nil, err
			}
	*/

	// creating federated identity credential
	log.Info("Creating FederatedIdentityCredential")
	err = h.createFederatedIdentityCredential(ctx, client, identity.Name, issuer)
	if err != nil {
		log.Error(err, "Failed to create FederatedIdentityCredential")
		return nil, err
	}

	// adding claim mapping policy

	return map[string]string{
		StatusMetaAegisIdentityObjectID: h.objectID,
		StatusMetaAegisIdentityID:       h.clientID,
		StatusMetaAegisAzureTenantID:    h.tenantID,
		StatusMetaAegisProvider:         ProviderName,
	}, nil
}

// New method to check and create app registration
func (h *IdentityHelper) ensureAppRegistration(ctx context.Context, client *msgraphsdk.GraphServiceClient, identity *aegisv1.Identity, issuer string) error {
	log := log.FromContext(ctx)

	filterQuery := fmt.Sprintf("displayName eq '%s'", h.subject)

	queryParameters := &applications.ApplicationsRequestBuilderGetQueryParameters{
		Filter: &filterQuery,
	}
	requestConfig := &applications.ApplicationsRequestBuilderGetRequestConfiguration{
		QueryParameters: queryParameters,
	}

	log.Info("Checking if application already exists")
	result, err := client.Applications().Get(ctx, requestConfig)
	if err != nil {
		log.Error(err, "Failed to list applications")
		return err
	}

	mustCreate := false
	apps := result.GetValue()
	if len(apps) == 0 {
		log.Info("No applications found", "filter", filterQuery)
		mustCreate = true
	}

	if mustCreate {
		app := models.NewApplication()
		app.SetDisplayName(&h.subject)
		app.SetTags([]string{
			"aegis",
			fmt.Sprintf("identity:%s", identity.Name),
			fmt.Sprintf("identity.namespace:%s", identity.Namespace),
			fmt.Sprintf("issuer:%s", issuer),
		})
		acceptMappedClaims := true
		api := models.NewApiApplication()
		api.SetAcceptMappedClaims(&acceptMappedClaims)
		app.SetApi(api)

		roleID := uuid.New()
		h.roleID = &roleID
		roles := models.NewAppRole()
		roleEnabled := true
		roles.SetId(h.roleID)
		roles.SetDisplayName(&h.subject)
		roles.SetValue(&h.subject)
		roles.SetAllowedMemberTypes([]string{"Application"})
		roles.SetDescription(&h.subject)
		roles.SetIsEnabled(&roleEnabled)
		app.SetAppRoles([]models.AppRoleable{roles})

		log.Info("Creating Azure Application")
		createdApp, err := client.Applications().Post(ctx, app, nil)
		if err != nil {
			log.Error(err, "Failed to create Azure Application")
			return err
		}
		h.objectID = *createdApp.GetId()
		h.clientID = *createdApp.GetAppId()

		servicePrincipal := models.NewServicePrincipal()
		servicePrincipal.SetAppId(&h.clientID)
		spResp, err := client.ServicePrincipals().Post(ctx, servicePrincipal, nil)
		if err != nil {
			log.Error(err, "Failed to create Azure ServicePrincipal")
			return err
		}
		h.servicePrincipalID = *spResp.GetId()

	} else {
		h.clientID = *apps[0].GetAppId()
		h.objectID = *apps[0].GetId()

		filterQuery := fmt.Sprintf("appId eq '%s'", h.clientID)
		queryParameters := &serviceprincipals.ServicePrincipalsRequestBuilderGetQueryParameters{
			Filter: &filterQuery,
		}
		requestConfig := &serviceprincipals.ServicePrincipalsRequestBuilderGetRequestConfiguration{
			QueryParameters: queryParameters,
		}
		spResp, err := client.ServicePrincipals().Get(ctx, requestConfig)
		if err != nil {
			log.Error(err, "Failed to get service principal")
			return err
		}
		h.servicePrincipalID = *spResp.GetValue()[0].GetId()
	}

	err = h.addAppRoleAssignment(ctx, client)
	if err != nil {
		log.Error(err, "Failed to add app role assignment")
		return err
	}
	// Call the new method to assign claims mapping policy to the service principal
	/*
		err = h.ensureAssignClaimsMappingPolicyToServicePrincipal(ctx, client)
		if err != nil {
			return err
		}
	*/

	return nil
}

func (h *IdentityHelper) addAppRoleAssignment(ctx context.Context, client *msgraphsdk.GraphServiceClient) error {
	log := log.FromContext(ctx)

	// check if the app role assignment already exists

	rolesAssigned, err := client.ServicePrincipals().ByServicePrincipalId(h.servicePrincipalID).AppRoleAssignedTo().Get(ctx, nil)
	if err != nil {
		log.Error(err, "Failed to get app role assignments")
		return err
	}
	if len(rolesAssigned.GetValue()) > 0 {
		log.Info("App role assignment already exists")
		h.roleID = rolesAssigned.GetValue()[0].GetAppRoleId()
		return nil
	}

	servicePrincipalIDUUID := uuid.MustParse(h.servicePrincipalID)
	appRoleAssignment := models.NewAppRoleAssignment()
	appRoleAssignment.SetPrincipalId(&servicePrincipalIDUUID)
	appRoleAssignment.SetResourceId(&servicePrincipalIDUUID)
	appRoleAssignment.SetAppRoleId(h.roleID)

	_, err = client.ServicePrincipals().ByServicePrincipalId(h.servicePrincipalID).AppRoleAssignments().Post(context.Background(), appRoleAssignment, nil)
	if err != nil {
		log.Error(err, "Failed to create app role assignment")
		return err
	}

	return nil
}

// New method to assign claims mapping policy to the service principal
func (h *IdentityHelper) ensureAssignClaimsMappingPolicyToServicePrincipal(ctx context.Context, client *msgraphsdk.GraphServiceClient) error {
	log := log.FromContext(ctx)

	cmID, err := h.getClaimsMappingPolicy(ctx, client)
	if err != nil {
		log.Error(err, "Failed to get claims mapping policy", "name", aegisClaimsMappingPolicyName)
		return err
	}
	log.Info("Found claims mapping policy", "policy", cmID)

	// Check if the claims mapping policy is already assigned to the service principal
	log.Info("Checking if claims mapping policy is already assigned", "servicePrincipalID", h.servicePrincipalID, "policy", cmID)
	assignedPolicies, err := client.ServicePrincipals().ByServicePrincipalId(h.servicePrincipalID).ClaimsMappingPolicies().Get(ctx, nil)
	if err != nil {
		log.Error(err, "Failed to get claims mapping policies for service principal")
		return err
	}

	for _, policy := range assignedPolicies.GetValue() {
		if *policy.GetId() == cmID {
			log.Info("Claims mapping policy is already assigned to the service principal", "servicePrincipalID", h.servicePrincipalID, "policy", cmID)
			return nil // Policy is already assigned, no action needed
		}
	}

	log.Info("Trying to assign service principal to claims mapping policy", "servicePrincipalID", h.servicePrincipalID, "policy", cmID)
	ref := models.NewReferenceCreate()
	refString := fmt.Sprintf("https://graph.microsoft.com/v1.0/policies/claimsMappingPolicies/%s", cmID)
	ref.SetOdataId(&refString)
	err = client.ServicePrincipals().ByServicePrincipalId(h.servicePrincipalID).ClaimsMappingPolicies().Ref().Post(ctx, ref, nil)
	if err != nil {
		log.Error(err, "Failed to set claims mapping policy to service principal", "policy", aegisClaimsMappingPolicyName)
		return err
	}

	return nil
}

func (h *IdentityHelper) getClaimsMappingPolicy(ctx context.Context, client *msgraphsdk.GraphServiceClient) (string, error) {
	log := log.FromContext(ctx)

	filterQuery := fmt.Sprintf("displayName eq '%s'", aegisClaimsMappingPolicyName)
	queryParameters := &policies.ClaimsMappingPoliciesRequestBuilderGetQueryParameters{
		Filter: &filterQuery,
	}
	requestOptions := &policies.ClaimsMappingPoliciesRequestBuilderGetRequestConfiguration{
		QueryParameters: queryParameters,
	}

	cresp, err := client.Policies().ClaimsMappingPolicies().Get(ctx, requestOptions)
	if err != nil {
		log.Error(err, "Failed to get claims mapping policy")
		return "", err
	}

	if len(cresp.GetValue()) == 0 {
		log.Info("Claims mapping policy not found", "desiredPolicyName", aegisClaimsMappingPolicyName)
		return "", fmt.Errorf("claims mapping policy not found: %s", aegisClaimsMappingPolicyName)
	}

	log.Info("Found claims mapping policy", "policy", cresp.GetValue()[0])
	return *cresp.GetValue()[0].GetId(), nil
}

func (h *IdentityHelper) createFederatedIdentityCredential(ctx context.Context,
	client *msgraphsdk.GraphServiceClient,
	name string,
	issuer string) error {

	log := log.FromContext(ctx)
	// Check if the federated identity credential already exists
	existingFICs, err := client.Applications().
		ByApplicationId(h.objectID).
		FederatedIdentityCredentials().
		Get(ctx, nil)
	if err != nil {
		log.Error(err, "Failed to list FederatedIdentityCredentials")
		return err
	}

	// Check if a FIC with matching name already exists
	for _, fic := range existingFICs.GetValue() {
		if *fic.GetName() == name {
			log.Info("FederatedIdentityCredential already exists", "name", name)
			return nil
		}
	}

	log.Info("Creating new FederatedIdentityCredential", "name", name)

	federatedIdentity := models.NewFederatedIdentityCredential()
	federatedIdentity.SetName(&name)
	federatedIdentity.SetIssuer(&issuer)
	federatedIdentity.SetSubject(&h.subject)
	federatedIdentity.SetAudiences([]string{"api://AzureADTokenExchange"})

	fiRequestBody := federatedIdentity

	_, err = client.
		Applications().ByApplicationId(h.objectID).
		FederatedIdentityCredentials().
		Post(ctx, fiRequestBody, nil)
	if err != nil {
		log.Error(err, "Failed to create FederatedIdentityCredential")
		return err
	}
	return nil
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

	appName := fmt.Sprintf("system:serviceaccount:%s:%s", identity.Namespace, identity.Name)

	filterQuery := fmt.Sprintf("displayName eq '%s'", appName)
	queryParameters := &applications.ApplicationsRequestBuilderGetQueryParameters{
		Filter: &filterQuery,
	}
	requestConfig := &applications.ApplicationsRequestBuilderGetRequestConfiguration{
		QueryParameters: queryParameters,
	}

	apps, err := client.Applications().Get(ctx, requestConfig)
	if err != nil {
		log.Error(err, "Failed to list applications")
		return err
	}

	var applicationID string
	if len(apps.GetValue()) > 0 {
		applicationID = *apps.GetValue()[0].GetId()
	} else {
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

// New method to create the extension property
func (h *IdentityHelper) createExtensionProperty(ctx context.Context, client *msgraphsdk.GraphServiceClient, exName string, extensionPropName string) error {
	log := log.FromContext(ctx)

	// Check if the extension already exists
	log.Info("Checking if directory extension already exists", "extension", extensionPropName, "objectID", h.objectID)
	extensionExists := false
	extensions, err := client.Applications().ByApplicationId(h.objectID).ExtensionProperties().Get(ctx, nil)
	if err != nil {
		log.Error(err, "Failed to get directory extension")
		return err
	}
	log.Info("extensions list", "extensions", extensions)
	for _, ext := range extensions.GetValue() {
		log.Info("extension", "name", *ext.GetName())
		if *ext.GetName() == extensionPropName {
			log.Info("Directory extension already exists", "extension", extensionPropName)
			extensionExists = true
			break
		}
	}
	if !extensionExists {
		log.Info("Creating directory extension", "extension", exName)
		dataType := "String"
		extension := models.NewExtensionProperty()
		extension.SetName(&exName)
		extension.SetDataType(&dataType)
		extension.SetTargetObjects([]string{"Application", "User"})
		isMultiValued := false
		extension.SetIsMultiValued(&isMultiValued)
		exresp, err := client.Applications().ByApplicationId(h.objectID).ExtensionProperties().Post(ctx, extension, nil)
		if err != nil {
			log.Error(err, "Failed to create directory extension", "exresp", exresp)
			return err
		}
		log.Info("Directory extension created", "exresp", exresp)
	}

	return nil
}
