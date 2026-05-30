package federation

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestLeafClient_NewLeafClient(t *testing.T) {
	lc := NewLeafClient(LeafClientConfig{
		AgentID:           "agent-001",
		HubAddr:           "hub.example.com:9443",
		ReconnectSec:      5,
		ReportIntervalSec: 30,
		Logger:            zerolog.Nop(),
	})
	assert.NotNil(t, lc)
	assert.False(t, lc.IsConnected())
}

func TestLeafClient_HealthStatus(t *testing.T) {
	lc := NewLeafClient(LeafClientConfig{
		AgentID: "agent-001",
		HubAddr: "hub:9443",
		Logger:  zerolog.Nop(),
	})
	status := lc.HealthStatus()
	assert.Equal(t, false, status["connected"])
	assert.Equal(t, "hub:9443", status["hub_addr"])
}
