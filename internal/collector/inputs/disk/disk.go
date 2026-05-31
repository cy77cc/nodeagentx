package disk

import (
	"context"
	"fmt"
	"slices"

	"github.com/shirou/gopsutil/v4/disk"

	"github.com/cy77cc/opsagent/internal/collector"
)

func init() {
	collector.RegisterInput("disk", func() collector.Input {
		return &DiskInput{}
	})
}

// DiskInput gathers disk usage and partition statistics.
type DiskInput struct {
	mountPoints []string
}

func (d *DiskInput) Init(cfg map[string]any) error {
	if v, ok := cfg["mount_points"]; ok {
		switch mp := v.(type) {
		case []any:
			for _, item := range mp {
				s, ok := item.(string)
				if !ok {
					return fmt.Errorf("disk: mount_points items must be strings, got %T", item)
				}
				d.mountPoints = append(d.mountPoints, s)
			}
		case []string:
			d.mountPoints = mp
		default:
			return fmt.Errorf("disk: mount_points must be a list, got %T", v)
		}
	}
	return nil
}

func (d *DiskInput) Gather(ctx context.Context, acc collector.Accumulator) error {
	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return fmt.Errorf("disk: failed to get partitions: %w", err)
	}

	for _, p := range partitions {
		// Filter by mount points if configured
		if len(d.mountPoints) > 0 {
			found := slices.Contains(d.mountPoints, p.Mountpoint)
			if !found {
				continue
			}
		}

		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			return fmt.Errorf("disk: failed to get usage for %s: %w", p.Mountpoint, err)
		}

		tags := map[string]string{
			"mountpoint": p.Mountpoint,
			"device":     p.Device,
			"fstype":     p.Fstype,
		}
		fields := map[string]any{
			"total_bytes":  int64(usage.Total),
			"used_bytes":   int64(usage.Used),
			"free_bytes":   int64(usage.Free),
			"used_percent": usage.UsedPercent,
		}
		acc.AddGauge("disk", tags, fields)
	}

	return nil
}

func (d *DiskInput) SampleConfig() string {
	return `
  ## List of mount points to filter. Empty means all partitions.
  # mount_points = ["/", "/home"]
`
}
