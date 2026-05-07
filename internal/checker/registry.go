package checker

import (
	"slices"
	"sync"
)

// Registry holds registered Checker implementations keyed by type name.
type Registry struct {
	mu       sync.RWMutex
	checkers map[string]Checker
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{checkers: make(map[string]Checker)}
}

// Register adds a Checker to the registry, keyed by its Type().
func (r *Registry) Register(c Checker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkers[c.Type()] = c
}

// Get returns the Checker for the given type name, if registered.
func (r *Registry) Get(typ string) (Checker, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.checkers[typ]
	return c, ok
}

// Types returns the sorted list of registered type names.
func (r *Registry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.checkers))
	for k := range r.checkers {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// DefaultRegistry is the global checker registry.
var DefaultRegistry = NewRegistry()

// Register adds a Checker to the default registry.
func Register(c Checker) {
	DefaultRegistry.Register(c)
}
