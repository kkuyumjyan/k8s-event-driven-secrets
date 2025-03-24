package aws

import (
	"context"
	"encoding/json"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
)

// Ensure AWSSecretManagerProvider implements Provider
type AWSSecretManagerProvider struct {
	Region string
}

func (p *AWSSecretManagerProvider) StartListening(ctx context.Context, k8sClient client.Client, updatedSecrets *sync.Map) error {
	queueURL := os.Getenv("SQS_QUEUE_URL")
	listener := SQSListener{
		Client:   k8sClient, // Pass Kubernetes client
		QueueURL: queueURL,
	}
	go listener.StartListening(ctx, queueURL) // Run in Goroutine
	return nil
}

var awslogs = ctrl.Log.WithName("aws")

// ✅ Fetch a secret's value from AWS Secrets Manager
func (p *AWSSecretManagerProvider) FetchSecretData(ctx context.Context, secretPath string) (map[string][]byte, error) {
	log := ctrl.Log.WithName("awslogs.FetchSecretData")

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(p.Region))
	if err != nil {
		log.Error(err, "Unable to load AWS SDK config")
		return nil, err
	}

	client := secretsmanager.NewFromConfig(cfg)
	result, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretPath),
	})
	if err != nil {
		log.Error(err, "Failed to retrieve secret")
		return nil, err
	}

	// ✅ Convert JSON secret into a map[string][]byte directly
	secretData := make(map[string]string)
	if err := json.Unmarshal([]byte(*result.SecretString), &secretData); err != nil {
		log.Error(err, "Failed to parse AWS secret JSON")
	}

	// ✅ Convert to `map[string][]byte`
	finalSecretData := make(map[string][]byte)
	for key, value := range secretData {
		finalSecretData[key] = []byte(value)
	}

	return finalSecretData, nil
}

// ✅ Fetch the real user-visible name of an AWS Secret
func (p *AWSSecretManagerProvider) fetchSecretName(ctx context.Context, region, secretARN string) (string, error) {
	log := ctrl.Log.WithName("awslogs.FetchSecretData")

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		log.Error(err, "Unable to load AWS SDK config")
		return "", err
	}

	client := secretsmanager.NewFromConfig(cfg)
	result, err := client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretARN),
	})
	if err != nil {
		log.Error(err, "Failed to retrieve secret metadata")
		return "", err
	}

	return *result.Name, nil
}
