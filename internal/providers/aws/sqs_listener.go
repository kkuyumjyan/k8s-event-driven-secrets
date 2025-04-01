package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"sigs.k8s.io/controller-runtime/pkg/client"

	secretsv1alpha1 "github.com/kkuyumjyan/k8s-event-driven-secrets/api/v1alpha1"
	"github.com/kkuyumjyan/k8s-event-driven-secrets/internal/utils"
)

var sqslog = ctrl.Log.WithName("sqs")

// SQSListener listens for messages from AWS SQS and triggers reconciliations
type SQSListener struct {
	client.Client
	QueueURL          string
	SQSUpdatedSecrets *sync.Map
	Endpoint          string // Optional: used for local testing (e.g., LocalStack)
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
	log := sqslog.WithName("SQSListener.handleSQSEvent")

	log.Info("📩 Received SQS Event")

	// Parse the JSON message
	var event SQSMessage
	if err := json.Unmarshal([]byte(msg), &event); err != nil {
		log.Error(err, "❌ Failed to parse SQS message")
		return
	}

	// Extract region and secret name from the event
	region := event.Detail.AWSRegion
	fullSecretID := event.Detail.RequestParameters.SecretId // The ARN of secret returns here from AWS Event

	if fullSecretID == "" || region == "" {
		log.Info("⚠️ Missing secretId or region, skipping...")
		return
	}

	provider := &AWSSecretManagerProvider{
		Region:   region,
		Endpoint: s.Endpoint,
	}

	// ✅ Fetch the user-visible name instead of stripping
	secretPath, err := provider.fetchSecretName(ctx, region, fullSecretID)
	if err != nil {
		log.Error(err, "❌ Failed to fetch secret name", "fullSecretID", fullSecretID, "region", region)
		return
	}

	log.Info("🔄 Secret update detected", "AWSSecretID", fullSecretID, "SecretPath", secretPath)

	// Fetch the updated secret value
	secretValue, err := provider.FetchSecretData(ctx, secretPath)
	if err != nil {
		log.Error(err, "❌ Failed to fetch updated secret", "SecretPath", secretPath, "region", region)
		return
	}

	// ✅ Mask the secret before logging to prevent leaks
	maskedSecret := utils.MaskSecretData(secretValue)
	log.Info("✅ Successfully fetched updated secret", "AWSSecretID", fullSecretID, "MaskedSecret", maskedSecret)

	// Find all matching EventDrivenSecret resources in Kubernetes
	eventDrivenSecrets, err := s.findMatchingEventDrivenSecrets(ctx, secretPath)
	if err != nil {
		log.Error(err, "❌ Error finding EventDrivenSecret resources")
		return
	}

	if len(eventDrivenSecrets) == 0 {
		log.Info("❌ Error finding matching EventDrivenSecret resources", "SecretPath", secretPath)
	}

	// ✅ Update the EventDrivenSecret to trigger reconciliation
	utils.AnnotateEventDrivenSecrets(ctx, eventDrivenSecrets, s.Client)
}

// Find EventDrivenSecrets that reference the given AWS Secret ARN
func (s *SQSListener) findMatchingEventDrivenSecrets(ctx context.Context, secretPath string) ([]secretsv1alpha1.EventDrivenSecret, error) {
	log := sqslog.WithName("findMatchingEventDrivenSecrets")

	var eventDrivenSecretList secretsv1alpha1.EventDrivenSecretList
	err := s.List(ctx, &eventDrivenSecretList, client.MatchingFields{
		"spec.secretPath": secretPath,
	})
	if err != nil {
		log.Error(err, "Failed to list EventDrivenSecrets")
		return nil, err
	}

	return eventDrivenSecretList.Items, nil
}

// StartListening continuously polls AWS SQS for secret update events
func (s *SQSListener) StartListening(ctx context.Context, queueURL string) {
	log := sqslog.WithName("SQSListener.StartListening")

	// Read SQS Queue URL from ENV variable
	if queueURL == "" {
		log.Error(fmt.Errorf("SQS_QUEUE_URL is empty"), "❌ Missing SQS queue URL")
		return
	}
	s.QueueURL = queueURL

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Error(err, "❌ Failed to load AWS config")
		return
	}

	sqsClient := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		if s.Endpoint != "" {
			o.BaseEndpoint = aws.String(s.Endpoint)
			fmt.Println("Using custom endpoint:", s.Endpoint)
		}
	})

	for {
		// Fetch messages from SQS
		msgResult, err := sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(s.QueueURL),
			MaxNumberOfMessages: 5,
			WaitTimeSeconds:     10, // Long polling for efficiency
		})

		if err != nil {
			log.Error(err, "Error receiving SQS messages")
			time.Sleep(5 * time.Second)
			continue
		}

		for _, message := range msgResult.Messages {
			fmt.Println("Received SQS event:", *message.Body)

			// Process the message
			s.handleSQSEvent(ctx, *message.Body)

			// Delete message after processing
			_, err := sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(s.QueueURL),
				ReceiptHandle: message.ReceiptHandle,
			})
			if err != nil {
				log.Error(err, "❌ Failed to delete processed SQS message")
				return
			}
		}
	}
}
