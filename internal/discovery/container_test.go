package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerLayerName(t *testing.T) {
	layer := &ContainerLayer{}
	assert.Equal(t, "container", layer.Name())
}

func TestContainerLayerNoDocker(t *testing.T) {
	layer := &ContainerLayer{
		DockerSocket: "/nonexistent/docker.sock",
	}

	services, err := layer.Discover(context.Background())

	require.NoError(t, err)
	assert.Nil(t, services)
}

func TestContainerLayerWithMockDocker(t *testing.T) {
	containers := []dockerContainer{
		{
			ID:    "abc123def456abc123def456abc123de",
			Names: []string{"/nginx-proxy"},
			Image: "nginx:latest",
			State: "running",
			Ports: []dockerPort{
				{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
				{IP: "0.0.0.0", PrivatePort: 443, PublicPort: 8443, Type: "tcp"},
			},
			Labels: map[string]string{
				"com.example.project": "web",
			},
		},
		{
			ID:    "fed654cba321fed654cba321fed654cb",
			Names: []string{"/redis-cache"},
			Image: "redis:7",
			State: "running",
			Ports: []dockerPort{
				{PrivatePort: 6379, Type: "tcp"},
			},
			Labels: map[string]string{},
		},
		{
			ID:    "11111111111111111111111111111111",
			Names: []string{"/stopped-app"},
			Image: "alpine:3.19",
			State: "exited",
			Ports: nil,
			Labels: map[string]string{},
		},
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/containers/json", r.URL.Path)
		assert.Equal(t, `{"status":["running"]}`, r.URL.Query().Get("filters"))

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(containers); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer mockServer.Close()

	layer := &ContainerLayer{
		DockerSocket: "/nonexistent/docker.sock", // not used when client is injected
		dockerClient: mockServer.Client(),
		dockerURL:    mockServer.URL,
	}

	services, err := layer.Discover(context.Background())

	require.NoError(t, err)
	require.Len(t, services, 2, "should discover 2 running containers, skip 1 exited")

	// First container: nginx-proxy
	assert.Equal(t, "nginx-proxy", services[0].Name)
	assert.Equal(t, "container", services[0].Type)
	assert.Equal(t, []int{8080, 8443}, services[0].Ports)
	assert.Equal(t, "web", services[0].Labels["com.example.project"])
	assert.Equal(t, "nginx:latest", services[0].Labels["image"])
	assert.Equal(t, "abc123def456abc123def456abc123de", services[0].Labels["container_id"])

	// Second container: redis-cache (private port used when no public port mapped)
	assert.Equal(t, "redis-cache", services[1].Name)
	assert.Equal(t, "container", services[1].Type)
	assert.Equal(t, []int{6379}, services[1].Ports)
	assert.Equal(t, "redis:7", services[1].Labels["image"])
}

func TestContainerLayerEmptyResponse(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]")
	}))
	defer mockServer.Close()

	layer := &ContainerLayer{
		dockerClient: mockServer.Client(),
		dockerURL:    mockServer.URL,
	}

	services, err := layer.Discover(context.Background())

	require.NoError(t, err)
	assert.Empty(t, services)
}

func TestContainerLayerAPIError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "connection refused", http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	layer := &ContainerLayer{
		dockerClient: mockServer.Client(),
		dockerURL:    mockServer.URL,
	}

	_, err := layer.Discover(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestContainerLayerContextCancellation(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should never reach here if context is already cancelled
		t.Error("handler should not be called with cancelled context")
	}))
	defer mockServer.Close()

	layer := &ContainerLayer{
		dockerClient: mockServer.Client(),
		dockerURL:    mockServer.URL,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := layer.Discover(ctx)

	require.Error(t, err)
}
