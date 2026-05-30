package federation

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	LeavesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "opsagent_federation_leaves_total",
			Help: "Number of connected Leaf agents by status",
		},
		[]string{"region", "status"},
	)
	GroupsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "opsagent_federation_groups_total",
			Help: "Number of configured groups",
		},
		[]string{"region"},
	)
	OperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opsagent_federation_operations_total",
			Help: "Total batch operations by type and status",
		},
		[]string{"type", "status"},
	)
	OperationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "opsagent_federation_operation_duration_seconds",
			Help:    "Duration of batch operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"type"},
	)
	MetricsReceived = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opsagent_federation_metrics_received_total",
			Help: "Total metrics received from Leaf agents",
		},
		[]string{"region"},
	)
	MetricsForwarded = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opsagent_federation_metrics_forwarded_total",
			Help: "Total metrics forwarded to platform",
		},
		[]string{"region"},
	)
	ConfigDistributionDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "opsagent_federation_config_distribution_duration_seconds",
			Help:    "Duration of config distribution to Leaf agents",
			Buckets: prometheus.DefBuckets,
		},
	)
	HubConnected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "opsagent_leaf_hub_connected",
			Help: "Whether the Leaf is connected to Hub (1) or not (0)",
		},
		[]string{"hub_addr"},
	)
	FallbackActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "opsagent_leaf_fallback_active",
			Help: "Whether the Leaf is in fallback mode (1) or not (0)",
		},
	)
)

// RegisterMetrics registers all federation Prometheus metrics.
func RegisterMetrics() {
	prometheus.MustRegister(
		LeavesTotal,
		GroupsTotal,
		OperationsTotal,
		OperationDuration,
		MetricsReceived,
		MetricsForwarded,
		ConfigDistributionDuration,
		HubConnected,
		FallbackActive,
	)
}
