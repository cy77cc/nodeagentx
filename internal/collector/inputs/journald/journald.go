package journald

import (
	"context"
	"fmt"
	"time"

	"github.com/coreos/go-systemd/v22/sdjournal"

	"github.com/cy77cc/opsagent/internal/collector"
)

const sampleConfig = `
  ## Journald input plugin (Linux only, requires systemd)
  # units = ["nginx", "docker", "sshd"]
  # priority = "info"           # emerg..debug
  # cursor_persist_path = "/var/lib/opsagent/journal.cursor"
`

func init() {
	collector.RegisterInput("journald", func() collector.Input {
		return &JournaldInput{}
	})
}

// JournaldInput reads entries from the systemd journal.
type JournaldInput struct {
	Units             []string `toml:"units"`
	Priority          string   `toml:"priority"`
	CursorPersistPath string   `toml:"cursor_persist_path"`

	priorityVal int
}

// Init parses the config map and sets defaults.
func (j *JournaldInput) Init(cfg map[string]interface{}) error {
	j.Priority = "info"

	if cfg == nil {
		return nil
	}

	if v, ok := cfg["units"]; ok {
		arr, ok := v.([]interface{})
		if !ok {
			return fmt.Errorf("journald: units must be a list")
		}
		j.Units = make([]string, 0, len(arr))
		for _, item := range arr {
			s, ok := item.(string)
			if !ok {
				return fmt.Errorf("journald: unit must be a string")
			}
			j.Units = append(j.Units, s)
		}
	}

	if v, ok := cfg["priority"]; ok {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("journald: priority must be a string")
		}
		j.Priority = s
	}

	if v, ok := cfg["cursor_persist_path"]; ok {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("journald: cursor_persist_path must be a string")
		}
		j.CursorPersistPath = s
	}

	p, err := parsePriority(j.Priority)
	if err != nil {
		return err
	}
	j.priorityVal = p

	return nil
}

// Gather reads journal entries and emits them as metrics.
func (j *JournaldInput) Gather(ctx context.Context, acc collector.Accumulator) error {
	journal, err := sdjournal.NewJournal()
	if err != nil {
		return fmt.Errorf("journald: failed to open journal: %w", err)
	}
	defer journal.Close()

	// Add unit filters
	for _, unit := range j.Units {
		match := fmt.Sprintf("_SYSTEMD_UNIT=%s.service", unit)
		if err := journal.AddMatch(match); err != nil {
			return fmt.Errorf("journald: add match %q: %w", match, err)
		}
	}

	// Add priority filter
	if j.priorityVal > 0 {
		if err := journal.AddDisjunction(); err != nil {
			return err
		}
		for i := 0; i <= j.priorityVal; i++ {
			match := fmt.Sprintf("PRIORITY=%d", i)
			if err := journal.AddMatch(match); err != nil {
				return err
			}
		}
	}

	// Seek to end (only read new entries)
	if err := journal.SeekTail(); err != nil {
		return fmt.Errorf("journald: seek tail: %w", err)
	}
	// Wait briefly for any new entries
	journal.Wait(100 * time.Millisecond)

	count := 0
	for count < 1000 { // limit per gather
		if ctx.Err() != nil {
			return ctx.Err()
		}

		r, err := journal.Next()
		if err != nil {
			return err
		}
		if r == 0 {
			break // no more entries
		}

		entry, err := journal.GetEntry()
		if err != nil {
			continue
		}

		fields := map[string]interface{}{
			"message":   string(entry.Fields["MESSAGE"]),
			"timestamp": int64(entry.RealtimeTimestamp),
		}
		if pid, ok := entry.Fields["_PID"]; ok {
			fields["pid"] = string(pid)
		}
		if comm, ok := entry.Fields["_COMM"]; ok {
			fields["command"] = string(comm)
		}
		if unit, ok := entry.Fields["_SYSTEMD_UNIT"]; ok {
			fields["unit"] = string(unit)
		}
		if prio, ok := entry.Fields["PRIORITY"]; ok {
			fields["priority"] = string(prio)
		}

		tags := map[string]string{}
		if unit, ok := entry.Fields["_SYSTEMD_UNIT"]; ok {
			tags["unit"] = string(unit)
		}

		acc.AddGauge("journald", tags, fields)
		count++
	}

	return nil
}

// SampleConfig returns a sample configuration string.
func (j *JournaldInput) SampleConfig() string {
	return sampleConfig
}

// parsePriority converts a priority name to its numeric syslog value.
func parsePriority(s string) (int, error) {
	switch s {
	case "emerg":
		return 0, nil
	case "alert":
		return 1, nil
	case "crit":
		return 2, nil
	case "err":
		return 3, nil
	case "warning", "warn":
		return 4, nil
	case "notice":
		return 5, nil
	case "info":
		return 6, nil
	case "debug":
		return 7, nil
	default:
		return -1, fmt.Errorf("journald: unknown priority %q", s)
	}
}
