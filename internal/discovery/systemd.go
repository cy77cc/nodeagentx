package discovery

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// SystemdLayer discovers services managed by systemd.
type SystemdLayer struct{}

// Name returns "systemd".
func (s *SystemdLayer) Name() string {
	return "systemd"
}

// Discover runs systemctl to find running services and their PIDs.
func (s *SystemdLayer) Discover(ctx context.Context) ([]Service, error) {
	cmd := exec.CommandContext(ctx, "systemctl", "list-units", "--type=service", "--state=running", "--no-legend", "--no-pager", "--plain")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list systemd units: %w", err)
	}

	var services []Service
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		unitName := fields[0]

		if !strings.HasSuffix(unitName, ".service") {
			continue
		}

		pid, err := s.getUnitPID(ctx, unitName)
		if err != nil {
			continue
		}

		serviceName := strings.TrimSuffix(unitName, ".service")

		services = append(services, Service{
			Name:   serviceName,
			Type:   "systemd",
			PID:    pid,
			Labels: map[string]string{"unit": unitName},
		})
	}

	return services, nil
}

// getUnitPID gets the MainPID for a systemd unit.
func (s *SystemdLayer) getUnitPID(ctx context.Context, unitName string) (int, error) {
	cmd := exec.CommandContext(ctx, "systemctl", "show", unitName, "--property=MainPID", "--value")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get PID for %s: %w", unitName, err)
	}

	pidStr := strings.TrimSpace(string(output))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID %q for %s: %w", pidStr, unitName, err)
	}

	if pid == 0 {
		return 0, fmt.Errorf("unit %s has no PID", unitName)
	}

	return pid, nil
}
