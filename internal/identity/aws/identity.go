package aws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentity"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	aegisv1 "github.com/vmarchese/aegis-operator/api/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ProviderName = "aws"
	K8STokenPath = "/var/run/secrets/tokens/aws_token"

	awsProviderName = "aegis-operator"
	identityMetaID  = "aegis.identity.id"
)

type IdentityHelper struct {
	region         string
	roleARN        string
	identityPoolId string
	clientset      *kubernetes.Clientset
}

func New(region string, roleARN string, identityPoolId string, clientset *kubernetes.Clientset) *IdentityHelper {
	return &IdentityHelper{
		region:         region,
		identityPoolId: identityPoolId,
		clientset:      clientset,
		roleARN:        roleARN,
	}
}

func (h *IdentityHelper) GetName() string {
	return ProviderName
}

func (h *IdentityHelper) CreateIdentity(ctx context.Context, identity *aegisv1.Identity) (map[string]string, error) {
	log := log.FromContext(ctx)

	// get service account
	sa, err := h.clientset.CoreV1().ServiceAccounts(identity.Namespace).Get(ctx, identity.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service account: %v", err)
	}

	// create token request for service  account
	tr := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &[]int64{3600}[0],
			Audiences:         []string{"sts.amazonaws.com"},
		},
	}

	tokenRequest, err := h.clientset.CoreV1().ServiceAccounts(identity.Namespace).CreateToken(ctx, sa.Name, tr, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %v", err)
	}

	token := tokenRequest.Status.Token

	// Create a Cognito Identity client
	cognitoClient := cognitoidentity.NewFromConfig(aws.Config{
		Region: h.region,
	})

	issuer, err := h.GetIssuer(ctx)
	if err != nil {
		log.Error(err, "Failed to get issuer")
		return nil, err
	}

	providerName := strings.ReplaceAll(issuer, "https://", "")
	// Create the identity on Cognito
	input := &cognitoidentity.GetIdInput{

		IdentityPoolId: aws.String(h.identityPoolId), // Replace with your Identity Pool ID
		Logins: map[string]string{
			providerName: token,
		},
	}

	result, err := cognitoClient.GetId(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create identity on Cognito: %v", err)
	}

	return map[string]string{identityMetaID: *result.IdentityId}, nil
}

func (h *IdentityHelper) GetIdentity(ctx context.Context, identity *aegisv1.Identity) (bool, error) {

	return false, fmt.Errorf("not implemented")
}

func (h *IdentityHelper) DeleteIdentity(ctx context.Context, identity *aegisv1.Identity) error {
	log := log.FromContext(ctx)
	cfg := aws.Config{
		Region: h.region,
	}

	stsClient := sts.NewFromConfig(cfg)

	// Assume the role using the service account token
	provider := stscreds.NewWebIdentityRoleProvider(
		stsClient,
		h.roleARN,
		stscreds.IdentityTokenFile(K8STokenPath),
		func(o *stscreds.WebIdentityRoleOptions) {
			o.RoleSessionName = "k8s-service-account-session"
		})

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(aws.NewCredentialsCache(provider)),
		config.WithRegion(h.region),
	)
	if err != nil {
		log.Error(err, "Failed to create AWS session with temporary credentials")
		return err
	}

	// Create a Cognito Identity client
	cognitoClient := cognitoidentity.NewFromConfig(awsCfg)

	/*
		issuer, err := h.GetIssuer(ctx)
		if err != nil {
			log.Error(err, "Failed to get issuer")
			return err
		}

		providerName := strings.ReplaceAll(issuer, "https://", "")
		unlinkInput := &cognitoidentity.UnlinkIdentityInput{
			IdentityId: aws.String(identity.Status.Metadata[identityMetaID]),
			Logins: map[string]*string{
				providerName: aws.String(token),
			},
			LoginsToRemove: []*string{
				aws.String(providerName),
			},
		}
		unlinkOut, err := cognitoClient.UnlinkIdentity(unlinkInput)
		if err != nil {
			log.Error(err, "Failed to unlink identity")
			return err
		}

		log.Info("Unlinked identity from Cognito", "identityId", identity.Status.Metadata[identityMetaID], "output", unlinkOut)
	*/

	// Delete the identity from Cognito
	cinput := &cognitoidentity.DeleteIdentitiesInput{
		IdentityIdsToDelete: []string{
			identity.Status.Metadata[identityMetaID],
		},
	}
	log.Info("Deleting identity from Cognito", "identityId", cinput)

	out, err := cognitoClient.DeleteIdentities(ctx, cinput)
	if err != nil {
		log.Error(err, "Failed to delete identity from Cognito", "output", out)
		return fmt.Errorf("failed to delete identity from Cognito: %v", err)
	}

	log.Info("Identity deleted from Cognito", "identityId", identity.Status.Metadata[identityMetaID], "output", out)

	return nil
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
