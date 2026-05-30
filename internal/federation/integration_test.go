package federation

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_HubLeafRegistration(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod", Match: map[string]string{"env": "prod"}},
	})
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"collector": map[string]interface{}{"interval_seconds": 30},
		},
	}, ge)

	hub := NewHub(HubConfig{
		ListenAddr:   ":0",
		Region:       "us-east",
		MaxLeaves:    10,
		Groups:       []GroupRule{{Name: "prod", Match: map[string]string{"env": "prod"}}},
		ConfigLevels: ConfigLevels{Global: map[string]interface{}{"key": "value"}},
		Logger:       zerolog.Nop(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	resp, err := hub.GetServer().Register(ctx, &FedAgentRegistration{
		AgentId:  "agent-001",
		Hostname: "web-01",
		Ip:       "10.0.1.1",
		Labels:   map[string]string{"env": "prod"},
	})
	require.NoError(t, err)
	assert.True(t, resp.Accepted)
	assert.Contains(t, resp.AssignedGroups, "prod")
	assert.NotEmpty(t, resp.ConfigVersion)

	leaves := hub.GetServer().GetLeaves()
	assert.Len(t, leaves, 1)
	assert.Equal(t, "online", leaves["agent-001"].Status)

	hbResp, err := hub.GetServer().Heartbeat(ctx, &FedHeartbeatRequest{
		AgentId: "agent-001",
	})
	require.NoError(t, err)
	assert.True(t, hbResp.Ok)

	health := hub.HealthStatus()
	assert.Equal(t, "running", health["status"])
	assert.Equal(t, 1, health["leaves_total"])
	assert.Equal(t, 1, health["leaves_online"])

	_ = cd
}

func TestIntegration_GroupDynamicUpdate(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod", Match: map[string]string{"env": "prod"}},
		{Name: "staging", Match: map[string]string{"env": "staging"}},
	})

	leaf := &LeafState{
		AgentID:  "agent-001",
		Labels:   map[string]string{"env": "prod"},
		LastSeen: time.Now(),
	}

	ge.UpdateLeaf(leaf)
	assert.Contains(t, ge.GetGroupMembers("prod"), "agent-001")
	assert.Empty(t, ge.GetGroupMembers("staging"))

	leaf.Labels = map[string]string{"env": "staging"}
	ge.UpdateLeaf(leaf)
	assert.Contains(t, ge.GetGroupMembers("staging"), "agent-001")
	assert.NotContains(t, ge.GetGroupMembers("prod"), "agent-001")
}

func TestIntegration_ConfigInheritance(t *testing.T) {
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"collector": map[string]interface{}{
				"interval_seconds": 30,
				"inputs":           []interface{}{"cpu", "memory"},
			},
		},
		Regions: map[string]map[string]interface{}{
			"us-east": {"collector": map[string]interface{}{"interval_seconds": 15}},
		},
		Groups: map[string]map[string]interface{}{
			"prod-web": {"collector": map[string]interface{}{"inputs": []interface{}{"cpu", "memory", "net", "http"}}},
		},
		Agents: map[string]map[string]interface{}{
			"agent-001": {"collector": map[string]interface{}{"interval_seconds": 5}},
		},
	}, NewGroupEngine(nil))

	cfg, err := cd.ResolveConfig("agent-001", "us-east", []string{"prod-web"})
	require.NoError(t, err)

	collector := cfg["collector"].(map[string]interface{})
	assert.Equal(t, 5, collector["interval_seconds"])
	assert.Equal(t, []interface{}{"cpu", "memory", "net", "http"}, collector["inputs"])
}
