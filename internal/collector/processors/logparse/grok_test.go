package logparse

import "testing"

func TestBuiltinPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		input   string
		wantKey string
		wantVal string
	}{
		{
			name:    "IP pattern matches IPv4",
			pattern: `%{IP:client}`,
			input:   "192.168.1.1",
			wantKey: "client",
			wantVal: "192.168.1.1",
		},
		{
			name:    "NUMBER pattern matches integer",
			pattern: `%{NUMBER:count}`,
			input:   "42",
			wantKey: "count",
			wantVal: "42",
		},
		{
			name:    "NUMBER pattern matches float",
			pattern: `%{NUMBER:ratio}`,
			input:   "3.14",
			wantKey: "ratio",
			wantVal: "3.14",
		},
		{
			name:    "IPV4 pattern matches",
			pattern: `%{IPV4:src}`,
			input:   "10.0.0.1",
			wantKey: "src",
			wantVal: "10.0.0.1",
		},
		{
			name:    "HOSTNAME pattern matches",
			pattern: `%{HOSTNAME:host}`,
			input:   "web-server-01.example.com",
			wantKey: "host",
			wantVal: "web-server-01.example.com",
		},
		{
			name:    "LOGLEVEL pattern matches",
			pattern: `%{LOGLEVEL:level}`,
			input:   "ERROR",
			wantKey: "level",
			wantVal: "ERROR",
		},
		{
			name:    "UUID pattern matches",
			pattern: `%{UUID:request_id}`,
			input:   "550e8400-e29b-41d4-a716-446655440000",
			wantKey: "request_id",
			wantVal: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:    "QUOTEDSTRING pattern matches",
			pattern: `%{QUOTEDSTRING:msg}`,
			input:   `"hello world"`,
			wantKey: "msg",
			wantVal: `"hello world"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := NewGrok(tt.pattern, nil)
			if err != nil {
				t.Fatalf("NewGrok() error: %v", err)
			}
			matches, err := g.Match(tt.input)
			if err != nil {
				t.Fatalf("Match() error: %v", err)
			}
			got, ok := matches[tt.wantKey]
			if !ok {
				t.Fatalf("expected key %q in matches, got %v", tt.wantKey, matches)
			}
			if got != tt.wantVal {
				t.Errorf("matches[%q] = %q, want %q", tt.wantKey, got, tt.wantVal)
			}
		})
	}
}

func TestCustomPattern(t *testing.T) {
	custom := map[string]string{
		"SEVERITY": `(?:CRITICAL|WARNING|INFO)`,
	}
	pattern := `%{SEVERITY:severity} %{GREEDYDATA:message}`
	g, err := NewGrok(pattern, custom)
	if err != nil {
		t.Fatalf("NewGrok() error: %v", err)
	}
	matches, err := g.Match("WARNING disk usage high")
	if err != nil {
		t.Fatalf("Match() error: %v", err)
	}
	if matches["severity"] != "WARNING" {
		t.Errorf("severity = %q, want %q", matches["severity"], "WARNING")
	}
	if matches["message"] != "disk usage high" {
		t.Errorf("message = %q, want %q", matches["message"], "disk usage high")
	}
}

func TestGrokNoMatch(t *testing.T) {
	g, err := NewGrok(`%{IP:client}`, nil)
	if err != nil {
		t.Fatalf("NewGrok() error: %v", err)
	}
	_, err = g.Match("not-an-ip")
	if err == nil {
		t.Fatal("expected error on no match, got nil")
	}
}
