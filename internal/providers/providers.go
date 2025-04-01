package providers

import (
	"context"
	"fmt"
	"os"

	"github.com/kkuyumjyan/k8s-event-driven-secrets/api/v1alpha1"
	"github.com/kkuyumjyan/k8s-event-driven-secrets/internal/providers/aws"
	"github.com/kkuyumjyan/k8s-event-driven-secrets/internal/providers/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Provider interface {
	FetchSecretData(ctx context.Context, secretPath string) (map[string][]byte, error)
	StartListening(ctx context.Context, k8sClient client.Client) error
}

func NewProvider(cloudProvider string, options v1alpha1.CloudProviderOptions) (Provider, error) {
	switch cloudProvider {
	case "aws":
		if options.Region == "" {
			return nil, fmt.Errorf("region is required for the AWS provider")
		}
		provider := &aws.AWSSecretManagerProvider{
			Region:   options.Region,
			Endpoint: os.Getenv("AWS_ENDPOINT"),
		}
		return provider, nil
	case "gcp":
		if options.GCPProjectID == "" {
			return nil, fmt.Errorf("GCPProjectID is required for the GCP provider")
		}
		provider := &gcp.GCPSecretsProvider{
			ProjectID: options.GCPProjectID,
		}
		return provider, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cloudProvider)
	}
}
