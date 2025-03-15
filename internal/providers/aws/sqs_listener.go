package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"sigs.k8s.io/controller-runtime/pkg/client"

	secretsv1alpha1 "github.com/kkuyumjyan/k8s-event-driven-secrets/api/v1alpha1"
	utils "github.com/kkuyumjyan/k8s-event-driven-secrets/internal/utils"
)

// SQSListener listens for messages from AWS SQS and triggers reconciliations
type SQSListener struct {
	client.Client
	QueueURL string
}

// SQSMessage represents the event structure from AWS SecretsManager
type SQSMessage struct {
	Detail struct {
		EventName         string `json:"eventName"`
		AWSRegion         string `json:"awsRegion"` // 🆕 Extract AWS region
		RequestParameters struct {
			SecretId string `json:"secretId"`
		} `json:"requestParameters"`
	} `json:"detail"`
}

func (s *SQSListener) handleSQSEvent(ctx context.Context, msg string) {
	fmt.Println("Received SQS Event:", msg)

	// Parse the JSON message
	var event SQSMessage
	if err := json.Unmarshal([]byte(msg), &event); err != nil {
		log.Printf("Error: Failed to parse SQS message: %v", err)
		return
	}

	// Extract region and secret name from the event
	region := event.Detail.AWSRegion
	fullSecretID := event.Detail.RequestParameters.SecretId // The ARN of secret returns here from AWS Event

	if fullSecretID == "" || region == "" {
		log.Println("Missing secretId or region, skipping...")
		return
	}

	provider := &AWSSecretManagerProvider{}

	// ✅ Fetch the user-visible name instead of stripping
	secretPath, err := provider.FetchSecretName(region, fullSecretID)
	if err != nil {
		log.Printf("Failed to fetch secret name: %v", err)
		return
	}

	fmt.Printf("🔄 Secret updated: %s (real name: %s)\n", fullSecretID, secretPath)

	// Fetch the updated secret value
	secretValue, err := provider.FetchSecretData(region, secretPath)
	if err != nil {
		log.Printf("Failed to fetch updated secret: %v", err)
		return
	}

	fmt.Printf("✅ Successfully fetched updated secret (%s): %s\n", fullSecretID, secretValue)

	// Find all matching EventDrivenSecret resources in Kubernetes
	eventDrivenSecrets, err := s.findMatchingEventDrivenSecrets(ctx, secretPath)
	if err != nil {
		log.Printf("❌ Error finding EventDrivenSecret resources: %v", err)
		return
	}

	if len(eventDrivenSecrets) == 0 {
		log.Println("⚠️ No matching EventDrivenSecret resources found. Skipping update.")
		return
	}

	// Loop through all matching EventDrivenSecrets and update their corresponding Kubernetes Secrets
	for _, eds := range eventDrivenSecrets {
		err := utils.CreateOrUpdateK8sSecret(ctx, s.Client, eds.Namespace, eds.Spec.TargetSecretName, secretValue)
		if err != nil {
			log.Printf("❌ Failed to update Kubernetes Secret for EventDrivenSecret %s: %v", eds.Name, err)
		} else {
			fmt.Printf("✅ Kubernetes Secret '%s' updated successfully\n", eds.Spec.TargetSecretName)
		}
	}

	// Rollout the Deployments that reference the secret
	err = utils.RolloutDeploymentsSecretUpdate(ctx, s.Client, eventDrivenSecrets[0].Spec.TargetSecretName, eventDrivenSecrets[0].Namespace)
	if err != nil {
		fmt.Println("Failed to rollout deployments:", err)
		return
	}
}

// Find EventDrivenSecrets that reference the given AWS Secret ARN
func (s *SQSListener) findMatchingEventDrivenSecrets(ctx context.Context, secretPath string) ([]secretsv1alpha1.EventDrivenSecret, error) {
	var eventDrivenSecretList secretsv1alpha1.EventDrivenSecretList
	err := s.List(ctx, &eventDrivenSecretList, client.MatchingFields{
		"spec.secretPath": secretPath,
	})
	if err != nil {
		log.Printf("failed to list EventDrivenSecrets: %v", err)
		return nil, err
	}

	return eventDrivenSecretList.Items, nil
}

// StartListening continuously polls AWS SQS for secret update events
func (s *SQSListener) StartListening(ctx context.Context, queueURL string) {
	// Read SQS Queue URL from ENV variable
	if queueURL == "" {
		log.Fatal("SQS_QUEUE_URL environment variable is not set")
	}
	s.QueueURL = queueURL

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	sqsClient := sqs.NewFromConfig(cfg)

	for {
		// Fetch messages from SQS
		msgResult, err := sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(s.QueueURL),
			MaxNumberOfMessages: 5,
			WaitTimeSeconds:     10, // Long polling for efficiency
		})

		if err != nil {
			log.Printf("Error receiving SQS messages: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, message := range msgResult.Messages {
			fmt.Println("Received SQS event:", *message.Body)

			// Process the message
			s.handleSQSEvent(context.TODO(), *message.Body)

			// Delete message after processing
			_, err := sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(s.QueueURL),
				ReceiptHandle: message.ReceiptHandle,
			})
			if err != nil {
				log.Printf("Failed to delete processed SQS message: %v", err)
			}
		}
	}
}
