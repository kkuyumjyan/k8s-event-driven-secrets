package providers

import (
	"context"
	"fmt"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kkuyumjyan/k8s-event-driven-secrets/internal/providers/aws"
)

type Provider interface {
	FetchSecretData(region, secretPath string) (map[string][]byte, error)
	FetchSecretName(region, secretARN string) (string, error)
	StartListening(ctx context.Context, queueURL string) error
}

func FetchSecretData(region, secretPath, cloudProvider string) (map[string][]byte, error) {
	switch cloudProvider {
	case "aws":
		provider := &aws.AWSSecretManagerProvider{
			Region: region,
		}
		return provider.FetchSecretData(region, secretPath)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cloudProvider)
	}
}

// StartListening initializes the event listener for the given cloud provider
func StartListening(ctx context.Context, cloudProvider string, k8sClient client.Client) error {
	switch cloudProvider {
	case "aws":
		queueURL := os.Getenv("SQS_QUEUE_URL")
		listener := aws.SQSListener{
			Client:   k8sClient, // Pass Kubernetes client
			QueueURL: queueURL,
		}
		go listener.StartListening(ctx, queueURL) // Run in Goroutine
		return nil
	default:
		return fmt.Errorf("unsupported provider: %s", cloudProvider)
	}
}
