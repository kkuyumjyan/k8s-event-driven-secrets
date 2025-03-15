package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// Ensure AWSSecretManagerProvider implements Provider
type AWSSecretManagerProvider struct {
	Region string
}

// ✅ Fetch a secret's value from AWS Secrets Manager
func (p *AWSSecretManagerProvider) FetchSecretData(region, secretPath string) (map[string][]byte, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)
	result, err := client.GetSecretValue(context.TODO(), &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretPath),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve secret: %w", err)
	}

	// ✅ Convert JSON secret into a map[string][]byte directly
	secretData := make(map[string]string)
	if err := json.Unmarshal([]byte(*result.SecretString), &secretData); err != nil {
		return nil, fmt.Errorf("failed to parse AWS secret JSON: %w", err)
	}

	// ✅ Convert to `map[string][]byte`
	finalSecretData := make(map[string][]byte)
	for key, value := range secretData {
		finalSecretData[key] = []byte(value)
	}

	return finalSecretData, nil
}

// ✅ Fetch the real user-visible name of an AWS Secret
func (p *AWSSecretManagerProvider) FetchSecretName(region, secretARN string) (string, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return "", fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)
	result, err := client.DescribeSecret(context.TODO(), &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretARN),
	})
	if err != nil {
		return "", fmt.Errorf("failed to retrieve secret metadata: %w", err)
	}

	return *result.Name, nil
}
