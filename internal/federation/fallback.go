package federation

import "sync"

type FallbackManager struct {
	cfg    FallbackConfig
	mu     sync.RWMutex
	active bool
}

func NewFallbackManager(cfg FallbackConfig) *FallbackManager {
	return &FallbackManager{cfg: cfg}
}

func (fm *FallbackManager) IsActive() bool {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return fm.active
}

func (fm *FallbackManager) Activate() {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.active = true
}

func (fm *FallbackManager) Deactivate() {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.active = false
}

func (fm *FallbackManager) Config() FallbackConfig {
	return fm.cfg
}
