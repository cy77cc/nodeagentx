package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultDockerSocket = "/var/run/docker.sock"

// dockerContainer represents the Docker API response for a running container.
type dockerContainer struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Ports  []dockerPort      `json:"Ports"`
	Labels map[string]string `json:"Labels"`
}

// dockerPort represents a port mapping from the Docker API.
type dockerPort struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

// ContainerLayer discovers running Docker containers.
type ContainerLayer struct {
	DockerSocket string
	dockerClient *http.Client
	dockerURL    string
}

// Name returns the layer name.
func (c *ContainerLayer) Name() string {
	return "container"
}

// Discover queries the Docker daemon for running containers and returns them as services.
func (c *ContainerLayer) Discover(ctx context.Context) ([]Service, error) {
	socket := c.DockerSocket
	if socket == "" {
		socket = defaultDockerSocket
	}

	// If Docker socket doesn't exist, Docker is not available.
	// Skip this check when using injected test client.
	if c.dockerClient == nil || c.dockerURL == "" {
		if _, err := os.Stat(socket); os.IsNotExist(err) {
			return nil, nil
		}
	}

	client, baseURL, err := c.dockerConnection(socket)
	if err != nil {
		return nil, fmt.Errorf("container discovery: %w", err)
	}

	url := fmt.Sprintf("%s/containers/json?filters={\"status\":[\"running\"]}", baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("container discovery: create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("container discovery: request docker api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("container discovery: docker api returned status %d", resp.StatusCode)
	}

	var containers []dockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("container discovery: decode response: %w", err)
	}

	services := make([]Service, 0, len(containers))
	for _, dc := range containers {
		if dc.State != "running" {
			continue
		}

		name := dc.Name()
		if name == "" {
			name = dc.ID[:12]
		}

		ports := extractPublicPorts(dc.Ports)

		labels := make(map[string]string, len(dc.Labels)+2)
		for k, v := range dc.Labels {
			labels[k] = v
		}
		labels["image"] = dc.Image
		labels["container_id"] = dc.ID

		services = append(services, Service{
			Name:   name,
			Type:   "container",
			Ports:  ports,
			Labels: labels,
		})
	}

	return services, nil
}

// dockerConnection returns the HTTP client and base URL for talking to Docker.
// If dockerClient and dockerURL are set (test injection), those are used.
func (c *ContainerLayer) dockerConnection(socket string) (*http.Client, string, error) {
	if c.dockerClient != nil && c.dockerURL != "" {
		return c.dockerClient, c.dockerURL, nil
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.DialTimeout("unix", socket, 3*time.Second)
			},
		},
		Timeout: 10 * time.Second,
	}

	return client, "http://localhost", nil
}

// Name returns the container name, trimming the leading "/" that Docker prepends.
func (dc dockerContainer) Name() string {
	if len(dc.Names) == 0 {
		return ""
	}
	return strings.TrimPrefix(dc.Names[0], "/")
}

// extractPublicPorts returns the list of public ports exposed by a container.
// If no public port is mapped, the private port is used.
func extractPublicPorts(ports []dockerPort) []int {
	var result []int
	for _, p := range ports {
		if p.PublicPort > 0 {
			result = append(result, p.PublicPort)
		} else {
			result = append(result, p.PrivatePort)
		}
	}
	return result
}
