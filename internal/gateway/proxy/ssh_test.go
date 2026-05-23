package proxy

import (
	"testing"
)

func TestSSHClientBuildAuthPassword(t *testing.T) {
	c := NewSSHClient(SSHConfig{User: "root", Password: "pass", Port: 22})
	methods, err := c.buildAuth()
	if err != nil {
		t.Fatal(err)
	}
	if len(methods) != 1 {
		t.Fatalf("expected 1 auth method, got %d", len(methods))
	}
}

func TestSSHClientBuildAuthNoMethods(t *testing.T) {
	c := NewSSHClient(SSHConfig{User: "root", Port: 22})
	_, err := c.buildAuth()
	if err == nil {
		t.Fatal("expected error for no auth methods")
	}
}

func TestSSHClientBuildAuthKeyFile(t *testing.T) {
	// Test with non-existent key file.
	c := NewSSHClient(SSHConfig{User: "root", KeyFile: "/nonexistent/key", Port: 22})
	_, err := c.buildAuth()
	if err == nil {
		t.Fatal("expected error for missing key file")
	}
}
