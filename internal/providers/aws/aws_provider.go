package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Ensure AWSSecretManagerProvider implements Provider
type AWSSecretManagerProvider struct {
	Region   string
	Endpoint string // Optional: used for local testing (e.g., LocalStack)
}

var awslog = ctrl.Log.WithName("aws")

func (p *AWSSecretManagerProvider) StartListening(ctx context.Context, k8sClient client.Client) error {
	queueURL := os.Getenv("SQS_QUEUE_URL")
	listener := SQSListener{
		Client:   k8sClient, // Pass Kubernetes client
		QueueURL: queueURL,
		Endpoint: p.Endpoint,
	}
	go listener.StartListening(ctx, queueURL) // Run in Goroutine
	return nil
}

// ✅ Fetch a secret's value from AWS Secrets Manager
func (p *AWSSecretManagerProvider) FetchSecretData(ctx context.Context, secretPath string) (map[string][]byte, error) {
	log := awslog.WithName("awslogs.FetchSecretData")

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(p.Region))
	if err != nil {
		log.Error(err, "Unable to load AWS SDK config")
		return nil, err
	}

	client := secretsmanager.NewFromConfig(cfg, func(o *secretsmanager.Options) {
		if p.Endpoint != "" {
			o.BaseEndpoint = aws.String(p.Endpoint)
			fmt.Println("Using custom endpoint:", p.Endpoint)
		}
	})

	result, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretPath),
	})
	if err != nil {
		log.Error(err, "Failed to retrieve secret 123", "secretPath", secretPath, "endpoint", p.Endpoint)
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
	log := awslog.WithName("awslogs.FetchSecretData")

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		log.Error(err, "Unable to load AWS SDK config")
		return "", err
	}

	client := secretsmanager.NewFromConfig(cfg, func(o *secretsmanager.Options) {
		if p.Endpoint != "" {
			o.BaseEndpoint = aws.String(p.Endpoint)
			fmt.Println("Using custom endpoint:", p.Endpoint)
		}
	})

	result, err := client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretARN),
	})
	if err != nil {
		log.Error(err, "Failed to retrieve secret metadata")
		return "", err
	}

	return *result.Name, nil
}
