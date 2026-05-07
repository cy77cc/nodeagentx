package checker

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	mc := &mockChecker{}
	r.Register(mc)

	got, ok := r.Get("mock")
	require.True(t, ok)
	assert.Equal(t, mc, got)
}

func TestRegistryGetUnknown(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	assert.False(t, ok)
}

func TestRegistryTypes(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockChecker{})
	r.Register(&mockChecker2{})

	types := r.Types()
	assert.Len(t, types, 2)
	assert.Contains(t, types, "mock")
	assert.Contains(t, types, "mock2")
}

func TestDefaultRegistry(t *testing.T) {
	orig := DefaultRegistry
	defer func() { DefaultRegistry = orig }()

	DefaultRegistry = NewRegistry()
	Register(&mockChecker{})
	got, ok := DefaultRegistry.Get("mock")
	require.True(t, ok)
	assert.Equal(t, "mock", got.Type())
}

type mockChecker2 struct{}

func (m *mockChecker2) Type() string     { return "mock2" }
func (m *mockChecker2) Category() string { return "test" }
func (m *mockChecker2) Check(_ context.Context, _ json.RawMessage) (*CheckResult, error) {
	return &CheckResult{Status: StatusPass}, nil
}
