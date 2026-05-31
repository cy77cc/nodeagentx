package proxy

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHConfig holds SSH connection parameters.
type SSHConfig struct {
	User               string
	Password           string
	KeyFile            string
	Port               int
	KnownHostsFile     string // path to known_hosts file
	InsecureSkipVerify bool   // if true, skip host key verification (with warning)
}

// SSHClient manages SSH connections to internal hosts.
type SSHClient struct {
	cfg SSHConfig
}

// NewSSHClient creates an SSHClient.
func NewSSHClient(cfg SSHConfig) *SSHClient {
	return &SSHClient{cfg: cfg}
}

// Connect establishes an SSH connection to the given address.
func (c *SSHClient) Connect(ctx context.Context, addr string) (*ssh.Client, error) {
	auth, err := c.buildAuth()
	if err != nil {
		return nil, fmt.Errorf("ssh auth: %w", err)
	}

	addr = fmt.Sprintf("%s:%d", addr, c.cfg.Port)

	hostKeyCallback, err := c.buildHostKeyCallback()
	if err != nil {
		return nil, fmt.Errorf("ssh host key: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            c.cfg.User,
		Auth:            auth,
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	// Use dialer with context.
	conn, err := (&net.Dialer{Timeout: config.Timeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ssh handshake %s: %w", addr, err)
	}

	return ssh.NewClient(sshConn, chans, reqs), nil
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// Execute runs a command on an SSH client and returns the result.
func (c *SSHClient) Execute(ctx context.Context, client *ssh.Client, command string, args []string) (exitCode int, stdout, stderr []byte, timedOut bool) {
	session, err := client.NewSession()
	if err != nil {
		return -1, nil, []byte(err.Error()), false
	}
	defer session.Close()

	var outBuf, errBuf bytes.Buffer
	session.Stdout = &outBuf
	session.Stderr = &errBuf

	var fullCmd strings.Builder
	fullCmd.WriteString(shellQuote(command))
	for _, arg := range args {
		fullCmd.WriteString(" " + shellQuote(arg))
	}

	done := make(chan error, 1)
	go func() {
		done <- session.Run(fullCmd.String())
	}()

	select {
	case <-ctx.Done():
		session.Signal(ssh.SIGKILL)
		return -1, outBuf.Bytes(), errBuf.Bytes(), true
	case err := <-done:
		if err != nil {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				return exitErr.ExitStatus(), outBuf.Bytes(), errBuf.Bytes(), false
			}
			return -1, outBuf.Bytes(), []byte(err.Error()), false
		}
		return 0, outBuf.Bytes(), errBuf.Bytes(), false
	}
}

func (c *SSHClient) buildAuth() ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if c.cfg.Password != "" {
		methods = append(methods, ssh.Password(c.cfg.Password))
	}

	if c.cfg.KeyFile != "" {
		key, err := os.ReadFile(c.cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no ssh auth methods configured (set password or key_file)")
	}

	return methods, nil
}

func (c *SSHClient) buildHostKeyCallback() (ssh.HostKeyCallback, error) {
	if c.cfg.InsecureSkipVerify {
		return ssh.InsecureIgnoreHostKey(), nil
	}

	knownHostsFile := c.cfg.KnownHostsFile
	if knownHostsFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("determine home directory: %w", err)
		}
		knownHostsFile = filepath.Join(home, ".ssh", "known_hosts")
	}

	callback, err := knownhosts.New(knownHostsFile)
	if err != nil {
		return nil, fmt.Errorf("load known hosts %s: %w", knownHostsFile, err)
	}
	return callback, nil
}
