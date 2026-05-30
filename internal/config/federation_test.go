package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFederationConfig_Validation_HubMode_RequiresListenAddr(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ""

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federation.hub.listen_addr")
}

func TestFederationConfig_Validation_HubMode_RequiresRegion(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = ""

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federation.hub.region")
}

func TestFederationConfig_Validation_LeafMode_RequiresHubAddr(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "leaf"
	cfg.Federation.Enabled = true
	cfg.Federation.Leaf.HubAddr = ""

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federation.leaf.hub_addr")
}

func TestFederationConfig_Validation_HubMode_MaxLeavesPositive(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = "us-east"
	cfg.Federation.Hub.MaxLeaves = 0

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federation.hub.max_leaves")
}

func TestFederationConfig_Validation_HubMode_ValidConfig(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = "us-east"
	cfg.Federation.Hub.MaxLeaves = 500
	cfg.Federation.Hub.LeafHeartbeatTimeoutSeconds = 60

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestFederationConfig_Validation_LeafMode_ValidConfig(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "leaf"
	cfg.Federation.Enabled = true
	cfg.Federation.Leaf.HubAddr = "hub.example.com:9443"

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestFederationConfig_Validation_DisabledFederation_SkipsChecks(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = false

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestFederationConfig_Validation_StandaloneMode_SkipsChecks(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "standalone"
	cfg.Federation.Enabled = true

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestFederationConfig_Validation_HubCanaryStages(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = "us-east"
	cfg.Federation.Hub.MaxLeaves = 100
	cfg.Federation.Hub.LeafHeartbeatTimeoutSeconds = 60
	cfg.Federation.Hub.Canary.Stages = []CanaryStageConfig{
		{Percentage: 150, WaitSeconds: 60, AutoRollback: true},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "percentage")
}

func TestFederationConfig_Validation_HubCanaryStages_NotSorted(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = "us-east"
	cfg.Federation.Hub.MaxLeaves = 100
	cfg.Federation.Hub.LeafHeartbeatTimeoutSeconds = 60
	cfg.Federation.Hub.Canary.Stages = []CanaryStageConfig{
		{Percentage: 50, WaitSeconds: 60},
		{Percentage: 10, WaitSeconds: 60},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sorted")
}

func defaultValidConfig() *Config {
	return &Config{
		Agent: AgentConfig{
			ID:                     "test-agent",
			Name:                   "test",
			IntervalSeconds:        10,
			ShutdownTimeoutSeconds: 30,
		},
		Server:     ServerConfig{ListenAddr: "127.0.0.1:18080"},
		Executor:   ExecutorConfig{TimeoutSeconds: 10, AllowedCommands: []string{"echo"}, MaxOutputBytes: 65536},
		Reporter:   ReporterConfig{Mode: "stdout", TimeoutSeconds: 5, RetryCount: 3, RetryIntervalMS: 500},
		Auth:       AuthConfig{Enabled: false},
		Prometheus: PrometheusConfig{Enabled: true, Path: "/metrics"},
		GRPC:       GRPCConfig{ServerAddr: "localhost:443", HeartbeatIntervalSeconds: 15, ReconnectInitialBackoffMS: 1000, ReconnectMaxBackoffMS: 30000},
	}
}
