package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFederationConfig_Validation_HubMode_RequiresListenAddr(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ""
	cfg.Federation.Hub.Region = "us-east"
	cfg.Federation.Hub.MaxLeaves = 500
	cfg.Federation.Hub.LeafHeartbeatTimeoutSeconds = 60
	cfg.Federation.Hub.MetricsAggregationIntervalSec = 30

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federation.hub.listen_addr")
}

func TestFederationConfig_Validation_HubMode_RequiresRegion(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = ""
	cfg.Federation.Hub.MaxLeaves = 500
	cfg.Federation.Hub.LeafHeartbeatTimeoutSeconds = 60
	cfg.Federation.Hub.MetricsAggregationIntervalSec = 30

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federation.hub.region")
}

func TestFederationConfig_Validation_LeafMode_RequiresHubAddr(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Agent.Mode = "leaf"
	cfg.Federation.Enabled = true
	cfg.Federation.Leaf.HubAddr = ""

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federation.leaf.hub_addr")
}

func TestFederationConfig_Validation_HubMode_MaxLeavesPositive(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = "us-east"
	cfg.Federation.Hub.MaxLeaves = 0
	cfg.Federation.Hub.LeafHeartbeatTimeoutSeconds = 60
	cfg.Federation.Hub.MetricsAggregationIntervalSec = 30

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federation.hub.max_leaves")
}

func TestFederationConfig_Validation_HubMode_ValidConfig(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = "us-east"
	cfg.Federation.Hub.MaxLeaves = 500
	cfg.Federation.Hub.LeafHeartbeatTimeoutSeconds = 60
	cfg.Federation.Hub.MetricsAggregationIntervalSec = 30

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestFederationConfig_Validation_LeafMode_ValidConfig(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Agent.Mode = "leaf"
	cfg.Federation.Enabled = true
	cfg.Federation.Leaf.HubAddr = "hub.example.com:9443"

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestFederationConfig_Validation_DisabledFederation_SkipsChecks(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = false

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestFederationConfig_Validation_StandaloneMode_SkipsChecks(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Agent.Mode = "standalone"
	cfg.Federation.Enabled = true

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestFederationConfig_Validation_HubCanaryStages(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = "us-east"
	cfg.Federation.Hub.MaxLeaves = 100
	cfg.Federation.Hub.LeafHeartbeatTimeoutSeconds = 60
	cfg.Federation.Hub.MetricsAggregationIntervalSec = 30
	cfg.Federation.Hub.Canary.Stages = []CanaryStageConfig{
		{Percentage: 150, WaitSeconds: 60, AutoRollback: true},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "percentage")
}

func TestFederationConfig_Validation_HubCanaryStages_NotSorted(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = "us-east"
	cfg.Federation.Hub.MaxLeaves = 100
	cfg.Federation.Hub.LeafHeartbeatTimeoutSeconds = 60
	cfg.Federation.Hub.MetricsAggregationIntervalSec = 30
	cfg.Federation.Hub.Canary.Stages = []CanaryStageConfig{
		{Percentage: 50, WaitSeconds: 60},
		{Percentage: 10, WaitSeconds: 60},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sorted")
}

func TestFederationConfig_Validation_HubMode_MetricsAggregationIntervalPositive(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = "us-east"
	cfg.Federation.Hub.MaxLeaves = 500
	cfg.Federation.Hub.LeafHeartbeatTimeoutSeconds = 60
	cfg.Federation.Hub.MetricsAggregationIntervalSec = 0

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metrics_aggregation_interval_seconds")
}

func TestFederationConfig_Validation_LeafMode_HubAddrWhitespace(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Agent.Mode = "leaf"
	cfg.Federation.Enabled = true
	cfg.Federation.Leaf.HubAddr = "   "

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federation.leaf.hub_addr")
}
