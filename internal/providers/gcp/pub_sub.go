package gcp

import (
	"context"
	"sync"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var pubsublog = ctrl.Log.WithName("pubsub")

type PubSubListener struct {
	client.Client
	QueueURL       string
	UpdatedSecrets *sync.Map
}

func (s *PubSubListener) StartListening(ctx context.Context, queueURL string) {
	log := pubsublog.WithName("PubSubListener.StartListening")

	log.Info("📩 Connecting to GCP Pub/Sub", "queueURL", queueURL)
}
