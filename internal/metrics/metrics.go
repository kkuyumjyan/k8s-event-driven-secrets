package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	TotalEventDrivenSecrets = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "edsm_total_eventdrivensecrets",
			Help: "Total number of EventDrivenSecret resources",
		},
		[]string{"namespace"},
	)

	DeploymentsUsingSecrets = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "edsm_deployments_using_secrets",
			Help: "Number of deployments using event-driven secrets",
		},
		[]string{"secret", "namespace"},
	)

	SecretsFetched = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "edsm_secret_fetched_total",
			Help: "Number of times secrets were fetched from the cloud provider",
		},
		[]string{"secret", "namespace"},
	)

	SecretsProvisionedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "edsm_secret_created_total",
			Help: "Number of times secrets were created in the cluster",
		},
		[]string{"secret", "namespace"},
	)

	DeploymentsRolledOut = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "edsm_deployment_rollout_total",
			Help: "Number of times deployments were rolled out due to secret updates",
		},
		[]string{"secret", "namespace"},
	)

	SecretsUpdatedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "edsm_secret_updated_total",
			Help: "Number of times secrets were updated in the cluster",
		},
		[]string{"secret", "namespace"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		TotalEventDrivenSecrets,
		DeploymentsUsingSecrets,
		SecretsFetched,
		SecretsProvisionedTotal,
		DeploymentsRolledOut,
		SecretsUpdatedTotal,
	)
}
