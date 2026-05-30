package federation

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestHub_New(t *testing.T) {
	hub := NewHub(HubConfig{
		ListenAddr: ":9443",
		Region:     "us-east",
		MaxLeaves:  100,
		Logger:     zerolog.Nop(),
	})
	assert.NotNil(t, hub)
}

func TestHub_HealthStatus_Stopped(t *testing.T) {
	hub := NewHub(HubConfig{
		ListenAddr: ":9443",
		Region:     "us-east",
		MaxLeaves:  100,
		Logger:     zerolog.Nop(),
	})
	status := hub.HealthStatus()
	assert.Equal(t, "stopped", status["status"])
}
