package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationManager_Create(t *testing.T) {
	om := NewOperationManager()
	op, err := om.Create("config_update", "prod-web", map[string]string{"key": "value"})
	require.NoError(t, err)
	assert.NotEmpty(t, op.ID)
	assert.Equal(t, "config_update", op.Type)
	assert.Equal(t, "prod-web", op.TargetGroup)
	assert.Equal(t, "pending", op.Status)
}

func TestOperationManager_GetStatus(t *testing.T) {
	om := NewOperationManager()
	op, _ := om.Create("restart", "prod-web", nil)
	status, err := om.GetStatus(op.ID)
	require.NoError(t, err)
	assert.Equal(t, op.ID, status.ID)
	assert.Equal(t, "pending", status.Status)
}

func TestOperationManager_GetStatus_NotFound(t *testing.T) {
	om := NewOperationManager()
	_, err := om.GetStatus("nonexistent")
	assert.Error(t, err)
}
