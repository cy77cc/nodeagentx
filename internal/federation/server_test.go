package federation

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHubServer_Register_AcceptsValidLeaf(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod", Match: map[string]string{"env": "prod"}},
	})
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]any{"key": "value"},
	}, ge)

	srv := NewHubServer(HubServerConfig{
		Region:            "us-east",
		MaxLeaves:         100,
		GroupEngine:       ge,
		ConfigDistributor: cd,
		Logger:            zerolog.Nop(),
	})

	resp, err := srv.Register(context.Background(), &FedAgentRegistration{
		AgentId:  "agent-001",
		Hostname: "web-01",
		Ip:       "10.0.1.1",
		Labels:   map[string]string{"env": "prod"},
	})
	require.NoError(t, err)
	assert.True(t, resp.Accepted)
	assert.Contains(t, resp.AssignedGroups, "prod")
	assert.NotEmpty(t, resp.ConfigVersion)
}

func TestHubServer_Register_RejectsWhenFull(t *testing.T) {
	ge := NewGroupEngine(nil)
	cd := NewConfigDistributor(ConfigLevels{}, ge)

	srv := NewHubServer(HubServerConfig{
		Region:            "us-east",
		MaxLeaves:         1,
		GroupEngine:       ge,
		ConfigDistributor: cd,
		Logger:            zerolog.Nop(),
	})

	_, err := srv.Register(context.Background(), &FedAgentRegistration{AgentId: "agent-001"})
	require.NoError(t, err)

	resp, err := srv.Register(context.Background(), &FedAgentRegistration{AgentId: "agent-002"})
	require.NoError(t, err)
	assert.False(t, resp.Accepted)
	assert.Contains(t, resp.RejectionReason, "full")
}

func TestHubServer_Heartbeat_UpdatesLeaf(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod", Match: map[string]string{"env": "prod"}},
	})
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]any{"key": "value"},
	}, ge)

	srv := NewHubServer(HubServerConfig{
		Region:            "us-east",
		MaxLeaves:         100,
		GroupEngine:       ge,
		ConfigDistributor: cd,
		Logger:            zerolog.Nop(),
	})

	srv.Register(context.Background(), &FedAgentRegistration{
		AgentId: "agent-001",
		Labels:  map[string]string{"env": "prod"},
	})

	resp, err := srv.Heartbeat(context.Background(), &FedHeartbeatRequest{
		AgentId: "agent-001",
	})
	require.NoError(t, err)
	assert.True(t, resp.Ok)
}

func TestHubServer_Heartbeat_UnknownAgent(t *testing.T) {
	srv := NewHubServer(HubServerConfig{
		Region:    "us-east",
		MaxLeaves: 100,
		Logger:    zerolog.Nop(),
	})

	resp, err := srv.Heartbeat(context.Background(), &FedHeartbeatRequest{
		AgentId: "unknown",
	})
	require.NoError(t, err)
	assert.False(t, resp.Ok)
}
