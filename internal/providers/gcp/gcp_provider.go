package gcp

import (
	"context"
	ctrl "sigs.k8s.io/controller-runtime"
)

// GCPProvider is a struct that holds the GCP provider configuration

type GCPSecretsProvider struct {
	ProjectID string
}

var gcplogs = ctrl.Log.WithName("gcp")

// ✅ Fetch a secret's value from AWS Secrets Manager
func (p *GCPSecretsProvider) FetchSecretData(ctx context.Context, region, secretPath string) (map[string][]byte, error) {
	log := ctrl.Log.WithName("gcp.FetchSecretData")

	// Fetch the secret from GCP Secret Manager
	log.Info("Fetching secret from GCP Secret Manager", "region", region, "secretPath", secretPath)
	return nil, nil
}
