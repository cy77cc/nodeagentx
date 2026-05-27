package tail

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Cursor stores the read position for a tailed file so the plugin can
// resume from where it left off after a restart.
type Cursor struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	Inode  uint64 `json:"inode"`
}

// Save persists the cursor to a JSON file at the given path.
// Parent directories are created if they do not exist.
func (c *Cursor) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create cursor directory: %w", err)
	}

	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal cursor: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write cursor file: %w", err)
	}

	return nil
}

// LoadCursor reads a cursor from a JSON file at the given path.
// Returns an error if the file does not exist or cannot be parsed.
func LoadCursor(path string) (*Cursor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cursor file: %w", err)
	}

	var c Cursor
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("unmarshal cursor: %w", err)
	}

	return &c, nil
}
