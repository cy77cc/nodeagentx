package discovery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// ProcLayer discovers services by inspecting /proc for LISTEN-state network connections.
type ProcLayer struct {
	procRoot string // filesystem root for /proc (default "/proc")
}

// NewProcLayer creates a new ProcLayer with the default /proc root.
func NewProcLayer() *ProcLayer {
	return &ProcLayer{procRoot: "/proc"}
}

// Name returns the layer name.
func (p *ProcLayer) Name() string {
	return "proc"
}

// Discover finds all LISTEN-state connections, groups by PID, and returns
// one Service per unique PID with the process name, ports, and cmdline.
func (p *ProcLayer) Discover(ctx context.Context) ([]Service, error) {
	conns, err := net.ConnectionsWithContext(ctx, "inet")
	if err != nil {
		return nil, fmt.Errorf("proc: failed to get connections: %w", err)
	}

	// Group listen ports by PID.
	type pidInfo struct {
		ports []int
	}
	pidMap := make(map[int32]*pidInfo)

	for _, c := range conns {
		if c.Status != "LISTEN" {
			continue
		}
		if c.Pid == 0 {
			continue
		}
		port := int(c.Laddr.Port)
		if port <= 0 {
			continue
		}
		info, ok := pidMap[c.Pid]
		if !ok {
			info = &pidInfo{}
			pidMap[c.Pid] = info
		}
		// Deduplicate ports within the same PID.
		if !containsPort(info.ports, port) {
			info.ports = append(info.ports, port)
		}
	}

	if len(pidMap) == 0 {
		return nil, nil
	}

	now := time.Now()
	var services []Service

	for pid, info := range pidMap {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		name, cmdline := p.procDetails(ctx, pid)

		svc := Service{
			Name:         name,
			Type:         "process",
			PID:          int(pid),
			Ports:        info.ports,
			Metadata:     map[string]any{"cmdline": cmdline},
			DiscoveredAt: now,
		}
		services = append(services, svc)
	}

	return services, nil
}

// procDetails retrieves the process name and command line for a given PID.
// It tries gopsutil first, then falls back to reading /proc/<pid>/comm for the name.
func (p *ProcLayer) procDetails(ctx context.Context, pid int32) (name, cmdline string) {
	proc, err := process.NewProcessWithContext(ctx, pid)
	if err == nil {
		if n, err := proc.NameWithContext(ctx); err == nil && n != "" {
			name = n
		}
		if cmd, err := proc.CmdlineWithContext(ctx); err == nil {
			cmdline = cmd
		}
	}

	// Fallback: read /proc/<pid>/comm
	if name == "" {
		name = p.readComm(pid)
	}

	if name == "" {
		name = fmt.Sprintf("pid-%d", pid)
	}

	return name, cmdline
}

// readComm reads the process name from /proc/<pid>/comm.
func (p *ProcLayer) readComm(pid int32) string {
	commPath := filepath.Join(p.procRoot, strconv.FormatInt(int64(pid), 10), "comm")
	data, err := os.ReadFile(commPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// containsPort checks whether a port slice already contains the given port.
func containsPort(ports []int, port int) bool {
	for _, p := range ports {
		if p == port {
			return true
		}
	}
	return false
}
