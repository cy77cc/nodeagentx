package tail

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/cy77cc/opsagent/internal/collector"
)

func init() {
	collector.RegisterInput("tail", func() collector.Input {
		return &TailInput{}
	})
}

// TailInput tails files and emits metrics line by line.
type TailInput struct {
	Files             []string `toml:"files"`
	WatchMethod       string   `toml:"watch_method"`       // poll | inotify
	FromBeginning     bool     `toml:"from_beginning"`
	CursorPersistPath string   `toml:"cursor_persist_path"`
	MaxLineBytes      int      `toml:"max_line_bytes"`

	mu      sync.Mutex
	offsets map[string]int64
}

// Init parses the config map and sets defaults.
func (t *TailInput) Init(cfg map[string]interface{}) error {
	// Defaults
	t.WatchMethod = "poll"
	t.MaxLineBytes = 65536
	t.offsets = make(map[string]int64)

	if v, ok := cfg["files"]; ok {
		fileSlice, ok := v.([]interface{})
		if !ok {
			return fmt.Errorf("tail: files must be a list, got %T", v)
		}
		t.Files = make([]string, 0, len(fileSlice))
		for _, f := range fileSlice {
			s, ok := f.(string)
			if !ok {
				return fmt.Errorf("tail: file entry must be a string, got %T", f)
			}
			t.Files = append(t.Files, s)
		}
	}

	if v, ok := cfg["watch_method"]; ok {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("tail: watch_method must be a string, got %T", v)
		}
		t.WatchMethod = s
	}

	if v, ok := cfg["from_beginning"]; ok {
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("tail: from_beginning must be a bool, got %T", v)
		}
		t.FromBeginning = b
	}

	if v, ok := cfg["cursor_persist_path"]; ok {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("tail: cursor_persist_path must be a string, got %T", v)
		}
		t.CursorPersistPath = s
	}

	if v, ok := cfg["max_line_bytes"]; ok {
		switch n := v.(type) {
		case int:
			t.MaxLineBytes = n
		case int64:
			t.MaxLineBytes = int(n)
		case float64:
			t.MaxLineBytes = int(n)
		default:
			return fmt.Errorf("tail: max_line_bytes must be a number, got %T", v)
		}
	}

	return nil
}

// Gather tails configured files and emits one metric per new line.
func (t *TailInput) Gather(ctx context.Context, acc collector.Accumulator) error {
	paths, err := t.expandGlobs()
	if err != nil {
		return fmt.Errorf("tail: expanding globs: %w", err)
	}

	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := t.gatherFile(path, acc); err != nil {
			return err
		}
	}

	return nil
}

// SampleConfig returns a sample configuration string.
func (t *TailInput) SampleConfig() string {
	return `
  ## Files to tail
  # files = ["/var/log/syslog"]
  ## Watch method: poll or inotify
  # watch_method = "poll"
  ## Read from beginning of file on first run
  # from_beginning = false
  ## Path to persist cursor (read offset)
  # cursor_persist_path = ""
  ## Maximum bytes per line before truncation
  # max_line_bytes = 65536
`
}

// expandGlobs expands glob patterns in the configured file list.
func (t *TailInput) expandGlobs() ([]string, error) {
	var result []string
	for _, pattern := range t.Files {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid glob %q: %w", pattern, err)
		}
		if matches == nil {
			// Treat as a literal path if no glob match
			result = append(result, pattern)
		} else {
			result = append(result, matches...)
		}
	}
	return result, nil
}

// gatherFile opens a file, seeks to the stored offset, reads new lines,
// and emits one metric per line via the accumulator.
func (t *TailInput) gatherFile(path string, acc collector.Accumulator) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("tail: open %s: %w", path, err)
	}
	defer f.Close()

	t.mu.Lock()
	offset := t.offsets[path]
	t.mu.Unlock()

	// If not from_beginning and this is the first run (offset==0), seek to end
	if !t.FromBeginning && offset == 0 {
		info, err := f.Stat()
		if err != nil {
			return fmt.Errorf("tail: stat %s: %w", path, err)
		}
		offset = info.Size()
		t.mu.Lock()
		t.offsets[path] = offset
		t.mu.Unlock()
		return nil
	}

	if _, err := f.Seek(offset, 0); err != nil {
		return fmt.Errorf("tail: seek %s: %w", path, err)
	}

	scanner := bufio.NewScanner(f)
	if t.MaxLineBytes > 0 {
		scanner.Buffer(make([]byte, 0, t.MaxLineBytes), t.MaxLineBytes)
	}

	for scanner.Scan() {
		line := scanner.Text()
		tags := map[string]string{"file": path}
		fields := map[string]interface{}{"message": line}
		acc.AddFields("tail", tags, fields)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("tail: scan %s: %w", path, err)
	}

	// Update offset to current position
	newOffset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("tail: tell %s: %w", path, err)
	}

	t.mu.Lock()
	t.offsets[path] = newOffset
	t.mu.Unlock()

	return nil
}
