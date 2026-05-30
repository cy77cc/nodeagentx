package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigDistributor_ResolveConfig_GlobalOnly(t *testing.T) {
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"collector": map[string]interface{}{"interval_seconds": 30},
		},
	}, NewGroupEngine(nil))
	cfg, err := cd.ResolveConfig("agent-001", "us-east", nil)
	require.NoError(t, err)
	assert.Equal(t, 30, cfg["collector"].(map[string]interface{})["interval_seconds"])
}

func TestConfigDistributor_ResolveConfig_RegionOverridesGlobal(t *testing.T) {
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"collector": map[string]interface{}{"interval_seconds": 30},
		},
		Regions: map[string]map[string]interface{}{
			"us-east": {"collector": map[string]interface{}{"interval_seconds": 15}},
		},
	}, NewGroupEngine(nil))
	cfg, err := cd.ResolveConfig("agent-001", "us-east", nil)
	require.NoError(t, err)
	assert.Equal(t, 15, cfg["collector"].(map[string]interface{})["interval_seconds"])
}

func TestConfigDistributor_ResolveConfig_GroupOverridesRegion(t *testing.T) {
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"collector": map[string]interface{}{"interval_seconds": 30},
		},
		Groups: map[string]map[string]interface{}{
			"prod-web": {"collector": map[string]interface{}{"interval_seconds": 10}},
		},
	}, NewGroupEngine(nil))
	cfg, err := cd.ResolveConfig("agent-001", "us-east", []string{"prod-web"})
	require.NoError(t, err)
	assert.Equal(t, 10, cfg["collector"].(map[string]interface{})["interval_seconds"])
}

func TestConfigDistributor_ResolveConfig_AgentOverridesGroup(t *testing.T) {
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"collector": map[string]interface{}{"interval_seconds": 30},
		},
		Groups: map[string]map[string]interface{}{
			"prod-web": {"collector": map[string]interface{}{"interval_seconds": 10}},
		},
		Agents: map[string]map[string]interface{}{
			"agent-001": {"collector": map[string]interface{}{"interval_seconds": 5}},
		},
	}, NewGroupEngine(nil))
	cfg, err := cd.ResolveConfig("agent-001", "us-east", []string{"prod-web"})
	require.NoError(t, err)
	assert.Equal(t, 5, cfg["collector"].(map[string]interface{})["interval_seconds"])
}

func TestConfigDistributor_ResolveConfig_ListReplacesNotMerges(t *testing.T) {
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"collector": map[string]interface{}{"inputs": []interface{}{"cpu", "memory"}},
		},
		Groups: map[string]map[string]interface{}{
			"prod-web": {"collector": map[string]interface{}{"inputs": []interface{}{"cpu", "memory", "net"}}},
		},
	}, NewGroupEngine(nil))
	cfg, err := cd.ResolveConfig("agent-001", "us-east", []string{"prod-web"})
	require.NoError(t, err)
	inputs := cfg["collector"].(map[string]interface{})["inputs"].([]interface{})
	assert.Equal(t, []interface{}{"cpu", "memory", "net"}, inputs)
}

func TestConfigDistributor_GetConfigVersion_SameConfigSameVersion(t *testing.T) {
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{"key": "value"},
	}, NewGroupEngine(nil))
	v1, _ := cd.GetConfigVersion("agent-001", "us-east", nil)
	v2, _ := cd.GetConfigVersion("agent-001", "us-east", nil)
	assert.Equal(t, v1, v2)
}
