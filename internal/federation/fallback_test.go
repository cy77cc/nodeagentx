package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFallbackManager_New(t *testing.T) {
	fm := NewFallbackManager(FallbackConfig{
		Enabled:          true,
		Mode:             "standalone",
		PlatformAddr:     "platform:443",
		CheckIntervalSec: 30,
	})
	assert.NotNil(t, fm)
	assert.False(t, fm.IsActive())
}

func TestFallbackManager_Activate(t *testing.T) {
	fm := NewFallbackManager(FallbackConfig{Enabled: true, Mode: "standalone"})
	fm.Activate()
	assert.True(t, fm.IsActive())
}

func TestFallbackManager_Deactivate(t *testing.T) {
	fm := NewFallbackManager(FallbackConfig{Enabled: true, Mode: "standalone"})
	fm.Activate()
	assert.True(t, fm.IsActive())
	fm.Deactivate()
	assert.False(t, fm.IsActive())
}
