package federation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// ConfigLevels holds configuration at multiple inheritance levels.
// Priority: Agent > Group > Region > Global
type ConfigLevels struct {
	Global  map[string]interface{}            `json:"global"`
	Regions map[string]map[string]interface{} `json:"regions"`
	Groups  map[string]map[string]interface{} `json:"groups"`
	Agents  map[string]map[string]interface{} `json:"agents"`
}

// ConfigDistributor resolves merged configuration for agents based on
// multi-level inheritance (global, region, group, agent).
type ConfigDistributor struct {
	levels ConfigLevels
	engine *GroupEngine
}

// NewConfigDistributor creates a new ConfigDistributor with the given levels and group engine.
func NewConfigDistributor(levels ConfigLevels, engine *GroupEngine) *ConfigDistributor {
	return &ConfigDistributor{levels: levels, engine: engine}
}

// ResolveConfig merges configuration levels for a specific agent.
// Priority order: Global < Region < Group < Agent
func (cd *ConfigDistributor) ResolveConfig(agentID, region string, groups []string) (map[string]interface{}, error) {
	result := deepCopyMap(cd.levels.Global)

	// Merge region config
	if regionCfg, ok := cd.levels.Regions[region]; ok {
		result = deepMerge(result, regionCfg)
	}

	// Merge group configs (sorted for deterministic order)
	if len(groups) > 0 {
		sortedGroups := make([]string, len(groups))
		copy(sortedGroups, groups)
		// Simple insertion sort for small slices
		for i := 0; i < len(sortedGroups); i++ {
			for j := i + 1; j < len(sortedGroups); j++ {
				if sortedGroups[i] > sortedGroups[j] {
					sortedGroups[i], sortedGroups[j] = sortedGroups[j], sortedGroups[i]
				}
			}
		}
		for _, g := range sortedGroups {
			if groupCfg, ok := cd.levels.Groups[g]; ok {
				result = deepMerge(result, groupCfg)
			}
		}
	}

	// Merge agent-specific config
	if agentCfg, ok := cd.levels.Agents[agentID]; ok {
		result = deepMerge(result, agentCfg)
	}

	return result, nil
}

// GetConfigVersion computes a deterministic hash of the resolved configuration.
func (cd *ConfigDistributor) GetConfigVersion(agentID, region string, groups []string) (string, error) {
	cfg, err := cd.ResolveConfig(agentID, region, groups)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:8]), nil
}

// UpdateLevels replaces the configuration levels (for hot-reload).
func (cd *ConfigDistributor) UpdateLevels(levels ConfigLevels) {
	cd.levels = levels
}

func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	result := deepCopyMap(dst)
	for k, srcVal := range src {
		dstVal, exists := result[k]
		if !exists {
			result[k] = deepCopy(srcVal)
			continue
		}
		srcMap, srcIsMap := srcVal.(map[string]interface{})
		dstMap, dstIsMap := dstVal.(map[string]interface{})
		if srcIsMap && dstIsMap {
			result[k] = deepMerge(dstMap, srcMap)
		} else {
			result[k] = deepCopy(srcVal)
		}
	}
	return result
}

func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return make(map[string]interface{})
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = deepCopy(v)
	}
	return result
}

func deepCopy(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return deepCopyMap(val)
	case []interface{}:
		cp := make([]interface{}, len(val))
		for i, item := range val {
			cp[i] = deepCopy(item)
		}
		return cp
	default:
		return v
	}
}
