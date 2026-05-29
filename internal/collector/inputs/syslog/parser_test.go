package syslog

import (
	"testing"
)

func TestParseRFC5424(t *testing.T) {
	line := `<13>1 2024-01-15T10:30:00.123456Z myhost nginx 1234 - - 192.168.1.1 GET /index.html 200`
	msg, err := Parse([]byte(line))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if msg.Facility != 1 {
		t.Errorf("Facility = %d, want 1", msg.Facility)
	}
	if msg.Severity != 5 {
		t.Errorf("Severity = %d, want 5", msg.Severity)
	}
	if msg.Hostname != "myhost" {
		t.Errorf("Hostname = %q, want myhost", msg.Hostname)
	}
	if msg.AppName != "nginx" {
		t.Errorf("AppName = %q, want nginx", msg.AppName)
	}
	if msg.ProcID != "1234" {
		t.Errorf("ProcID = %q, want 1234", msg.ProcID)
	}
	if msg.Message != "192.168.1.1 GET /index.html 200" {
		t.Errorf("Message = %q", msg.Message)
	}
}

func TestParseRFC3164(t *testing.T) {
	line := `<13>Jan 15 10:30:00 myhost nginx: GET /index.html 200`
	msg, err := Parse([]byte(line))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if msg.Hostname != "myhost" {
		t.Errorf("Hostname = %q, want myhost", msg.Hostname)
	}
	if msg.AppName != "nginx" {
		t.Errorf("AppName = %q, want nginx", msg.AppName)
	}
	if msg.Message != "GET /index.html 200" {
		t.Errorf("Message = %q", msg.Message)
	}
}

func TestParseInvalid(t *testing.T) {
	_, err := Parse([]byte("not a syslog message"))
	if err == nil {
		t.Error("expected error for invalid message")
	}
}

func TestParseRFC3164WithPID(t *testing.T) {
	line := `<13>Jan 15 10:30:00 myhost nginx[1234]: GET /index.html 200`
	msg, err := Parse([]byte(line))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if msg.AppName != "nginx" {
		t.Errorf("AppName = %q, want nginx", msg.AppName)
	}
	if msg.ProcID != "1234" {
		t.Errorf("ProcID = %q, want 1234", msg.ProcID)
	}
	if msg.Message != "GET /index.html 200" {
		t.Errorf("Message = %q", msg.Message)
	}
}

func TestParseRFC5424WithStructuredData(t *testing.T) {
	line := `<13>1 2024-01-15T10:30:00.123456Z myhost nginx 1234 msg123 [exampleSDID@32473 iut="3"] Hello World`
	msg, err := Parse([]byte(line))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if msg.MsgID != "msg123" {
		t.Errorf("MsgID = %q, want msg123", msg.MsgID)
	}
	if msg.Message != "Hello World" {
		t.Errorf("Message = %q, want Hello World", msg.Message)
	}
}

func TestParseEmpty(t *testing.T) {
	_, err := Parse([]byte(""))
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestParseUnclosedPriority(t *testing.T) {
	_, err := Parse([]byte("<13 missing close"))
	if err == nil {
		t.Error("expected error for unclosed priority")
	}
}

func TestParsePriority(t *testing.T) {
	tests := []struct {
		pri      int
		facility int
		severity int
	}{
		{13, 1, 5},   // user.notice
		{0, 0, 0},    // kern.emerg
		{165, 20, 5}, // local5.notice
	}
	for _, tt := range tests {
		f, s := decodePriority(tt.pri)
		if f != tt.facility || s != tt.severity {
			t.Errorf("decodePriority(%d) = (%d,%d), want (%d,%d)", tt.pri, f, s, tt.facility, tt.severity)
		}
	}
}
