package journald

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// maxGetEntryErrors is the number of consecutive GetEntry errors tolerated
// before aborting the gather loop.
const maxGetEntryErrors = 10

func init() {
	collector.RegisterInput("journald", func() collector.Input {
		return &JournaldInput{}
	})
}

// journalReader abstracts the sdjournal.Journal methods used by the plugin
// so tests can substitute a mock implementation.
type journalReader interface {
	Close() error
	AddMatch(match string) error
	AddDisjunction() error
	SeekTail() error
	SeekCursor(cursor string) error
	GetCursor() (string, error)
	Wait(timeout time.Duration) int
	Next() (uint64, error)
	GetEntry() (*sdjournal.JournalEntry, error)
}

// JournaldInput reads entries from the systemd journal.
type JournaldInput struct {
	Units             []string `toml:"units"`
	Priority          string   `toml:"priority"`
	CursorPersistPath string   `toml:"cursor_persist_path"`

	priorityVal int
	newJournal  func() (journalReader, error)
}

// Init parses the config map and sets defaults.
func (j *JournaldInput) Init(cfg map[string]any) error {
	j.Priority = "info"

	if j.newJournal == nil {
		j.newJournal = func() (journalReader, error) {
			return sdjournal.NewJournal()
		}
	}

	if cfg == nil {
		return nil
	}

	if v, ok := cfg["units"]; ok {
		arr, ok := v.([]any)
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
	journal, err := j.newJournal()
	if err != nil {
		return fmt.Errorf("journald: failed to open journal: %w", err)
	}
	defer journal.Close()

	// Add unit filters
	for _, unit := range j.Units {
		match := fmt.Sprintf("_SYSTEMD_UNIT=%s", unitSuffix(unit))
		if err := journal.AddMatch(match); err != nil {
			return fmt.Errorf("journald: add match %q: %w", match, err)
		}
	}

	// Add priority filter (always applied since Init sets a default)
	if err := journal.AddDisjunction(); err != nil {
		return fmt.Errorf("journald: add disjunction: %w", err)
	}
	for i := 0; i <= j.priorityVal; i++ {
		match := fmt.Sprintf("PRIORITY=%d", i)
		if err := journal.AddMatch(match); err != nil {
			return fmt.Errorf("journald: add match %q: %w", match, err)
		}
	}

	// Restore cursor or seek to tail on first run
	cursor, cursorErr := j.loadCursor()
	if cursorErr == nil && cursor != "" {
		if err := journal.SeekCursor(cursor); err != nil {
			// Cursor is stale or invalid; fall back to tail
			if err := journal.SeekTail(); err != nil {
				return fmt.Errorf("journald: seek tail: %w", err)
			}
		}
	} else {
		if err := journal.SeekTail(); err != nil {
			return fmt.Errorf("journald: seek tail: %w", err)
		}
	}

	// Wait briefly for any new entries
	journal.Wait(100 * time.Millisecond)

	var lastCursor string
	consecutiveErrors := 0
	count := 0

	for count < 1000 { // limit per gather
		if ctx.Err() != nil {
			return ctx.Err()
		}

		r, err := journal.Next()
		if err != nil {
			return fmt.Errorf("journald: next: %w", err)
		}
		if r == 0 {
			break // no more entries
		}

		// Get the cursor for this entry before reading it
		cur, curErr := journal.GetCursor()
		if curErr == nil {
			lastCursor = cur
		}

		entry, err := journal.GetEntry()
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= maxGetEntryErrors {
				return fmt.Errorf("journald: too many consecutive GetEntry errors (last: %w)", err)
			}
			continue
		}
		consecutiveErrors = 0

		ts := time.UnixMicro(int64(entry.RealtimeTimestamp))

		fields := map[string]any{
			"message": string(entry.Fields["MESSAGE"]),
		}
		if pid, ok := entry.Fields["_PID"]; ok {
			fields["pid"] = string(pid)
		}
		if comm, ok := entry.Fields["_COMM"]; ok {
			fields["command"] = string(comm)
		}
		if prio, ok := entry.Fields["PRIORITY"]; ok {
			fields["priority"] = string(prio)
		}

		tags := map[string]string{}
		if unit, ok := entry.Fields["_SYSTEMD_UNIT"]; ok {
			tags["unit"] = string(unit)
		}

		acc.AddGaugeWithTimestamp("journald", tags, fields, ts)
		count++
	}

	// Persist cursor for next Gather call
	if lastCursor != "" {
		if err := j.saveCursor(lastCursor); err != nil {
			return fmt.Errorf("journald: save cursor: %w", err)
		}
	}

	return nil
}

// SampleConfig returns a sample configuration string.
func (j *JournaldInput) SampleConfig() string {
	return sampleConfig
}

// unitSuffix appends ".service" only if the unit name does not already
// contain a dot (e.g. "nginx" -> "nginx.service", "docker.socket" stays).
func unitSuffix(unit string) string {
	if strings.Contains(unit, ".") {
		return unit
	}
	return unit + ".service"
}

// cursorFile is the JSON structure persisted to disk.
type cursorFile struct {
	Cursor string `json:"cursor"`
}

// loadCursor reads the saved cursor string from CursorPersistPath.
// Returns ("", nil) when no cursor file exists yet.
func (j *JournaldInput) loadCursor() (string, error) {
	if j.CursorPersistPath == "" {
		return "", nil
	}

	path := filepath.Join(j.CursorPersistPath, "journald.cursor")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read cursor file: %w", err)
	}

	var cf cursorFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return "", fmt.Errorf("unmarshal cursor: %w", err)
	}

	return cf.Cursor, nil
}

// saveCursor persists the cursor string to CursorPersistPath.
func (j *JournaldInput) saveCursor(cursor string) error {
	if j.CursorPersistPath == "" {
		return nil
	}

	if err := os.MkdirAll(j.CursorPersistPath, 0o755); err != nil {
		return fmt.Errorf("create cursor directory: %w", err)
	}

	cf := cursorFile{Cursor: cursor}
	data, err := json.Marshal(cf)
	if err != nil {
		return fmt.Errorf("marshal cursor: %w", err)
	}

	path := filepath.Join(j.CursorPersistPath, "journald.cursor")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write cursor file: %w", err)
	}

	return nil
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
