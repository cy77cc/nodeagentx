package templates

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoaderList(t *testing.T) {
	loader, err := NewLoader()
	require.NoError(t, err)
	require.NotNil(t, loader)

	names := loader.List()
	assert.GreaterOrEqual(t, len(names), 1, "expected at least 1 template")
	assert.Contains(t, names, "system", "expected 'system' template to exist")
}

func TestLoaderGet(t *testing.T) {
	loader, err := NewLoader()
	require.NoError(t, err)

	tmpl, err := loader.Get("nginx")
	require.NoError(t, err)
	require.NotNil(t, tmpl)

	assert.Equal(t, "nginx", tmpl.Name)
	assert.Equal(t, "Nginx web server monitoring", tmpl.Description)
	assert.Equal(t, "1.0.0", tmpl.Version)
	assert.NotEmpty(t, tmpl.Collector.Inputs, "expected at least 1 input")

	// Verify input types exist.
	types := make([]string, 0, len(tmpl.Collector.Inputs))
	for _, input := range tmpl.Collector.Inputs {
		types = append(types, input.Type)
	}
	assert.Contains(t, types, "http")
	assert.Contains(t, types, "tail")
}

func TestLoaderGetNotFound(t *testing.T) {
	loader, err := NewLoader()
	require.NoError(t, err)

	tmpl, err := loader.Get("nonexistent")
	assert.Error(t, err)
	assert.Nil(t, tmpl)
	assert.Contains(t, err.Error(), "not found")
}

func TestLoaderApply(t *testing.T) {
	loader, err := NewLoader()
	require.NoError(t, err)

	tmpl, err := loader.Get("nginx")
	require.NoError(t, err)

	vars := map[string]string{
		"stub_status_url": "http://10.0.0.1:9090/nginx_status",
		"log_path":        "/custom/nginx/access.log",
	}

	result, err := loader.Apply(tmpl, vars)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Inputs, 2, "expected 2 inputs")

	// Verify the http input has the substituted URL.
	httpInput := result.Inputs[0]
	assert.Equal(t, "http", httpInput.Type)
	urls, ok := httpInput.Config["urls"].([]interface{})
	require.True(t, ok, "expected urls to be a slice")
	require.Len(t, urls, 1)
	assert.Equal(t, "http://10.0.0.1:9090/nginx_status", urls[0])

	// Verify the tail input has the substituted log path.
	tailInput := result.Inputs[1]
	assert.Equal(t, "tail", tailInput.Type)
	files, ok := tailInput.Config["files"].([]interface{})
	require.True(t, ok, "expected files to be a slice")
	require.Len(t, files, 1)
	assert.Equal(t, "/custom/nginx/access.log", files[0])
}

func TestLoaderApplyDefaults(t *testing.T) {
	loader, err := NewLoader()
	require.NoError(t, err)

	tmpl, err := loader.Get("nginx")
	require.NoError(t, err)

	// Apply without any vars - should use defaults.
	result, err := loader.Apply(tmpl, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Inputs, 2)

	// Verify defaults are applied.
	httpInput := result.Inputs[0]
	urls, ok := httpInput.Config["urls"].([]interface{})
	require.True(t, ok)
	require.Len(t, urls, 1)
	assert.Equal(t, "http://127.0.0.1:80/nginx_status", urls[0])

	tailInput := result.Inputs[1]
	files, ok := tailInput.Config["files"].([]interface{})
	require.True(t, ok)
	require.Len(t, files, 1)
	assert.Equal(t, "/var/log/nginx/access.log", files[0])
}

func TestLoaderApplySystem(t *testing.T) {
	loader, err := NewLoader()
	require.NoError(t, err)

	tmpl, err := loader.Get("system")
	require.NoError(t, err)

	result, err := loader.Apply(tmpl, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Inputs, 5, "expected 5 system inputs")

	expectedTypes := []string{"cpu", "memory", "disk", "net", "load"}
	for i, expected := range expectedTypes {
		assert.Equal(t, expected, result.Inputs[i].Type)
	}
}
