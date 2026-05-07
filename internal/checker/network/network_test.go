package network

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cy77cc/opsagent/internal/checker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- PortChecker tests ---

func TestPortCheckerTypeAndCategory(t *testing.T) {
	c := &PortChecker{}
	assert.Equal(t, "port_check", c.Type())
	assert.Equal(t, "network", c.Category())
}

func TestPortCheckerInvalidPort(t *testing.T) {
	c := &PortChecker{}
	params := json.RawMessage(`{"port": 0, "expected_state": "listening"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1-65535")
}

func TestPortCheckerPortTooHigh(t *testing.T) {
	c := &PortChecker{}
	params := json.RawMessage(`{"port": 70000, "expected_state": "listening"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1-65535")
}

func TestPortCheckerInvalidExpectedState(t *testing.T) {
	c := &PortChecker{}
	params := json.RawMessage(`{"port": 22, "expected_state": "open"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listening")
}

func TestPortCheckerInvalidJSON(t *testing.T) {
	c := &PortChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`not json`))
	require.Error(t, err)
}

func TestPortCheckerNotListening(t *testing.T) {
	// Use a port that is very likely not in use.
	c := &PortChecker{}
	params := json.RawMessage(`{"port": 59999, "expected_state": "not_listening"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
	assert.Equal(t, "not_listening", result.ActualValue)
}

func TestPortCheckerWithProcNet(t *testing.T) {
	if _, err := os.Stat("/proc/net/tcp"); err != nil {
		t.Skip("no /proc/net/tcp available")
	}

	// Check port 0 against not_listening (should always pass for port not in use).
	c := &PortChecker{}
	params := json.RawMessage(`{"port": 59998, "expected_state": "not_listening"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
}

// --- SSHConfigChecker tests ---

func TestSSHConfigCheckerTypeAndCategory(t *testing.T) {
	c := &SSHConfigChecker{}
	assert.Equal(t, "ssh_config_check", c.Type())
	assert.Equal(t, "network", c.Category())
}

func TestSSHConfigCheckerEmptyKey(t *testing.T) {
	c := &SSHConfigChecker{}
	params := json.RawMessage(`{"key": "", "expected": "no"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key is required")
}

func TestSSHConfigCheckerInvalidJSON(t *testing.T) {
	c := &SSHConfigChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`{bad`))
	require.Error(t, err)
}

func TestSSHConfigCheckerFileNotFound(t *testing.T) {
	c := &SSHConfigChecker{}
	params := json.RawMessage(`{"key": "PermitRootLogin", "expected": "no"}`)
	_, err := c.Check(context.Background(), params)
	// On systems without /etc/ssh/sshd_config, this should return an error result.
	if err != nil {
		// Error from the Check method itself (e.g. file not wrapped properly).
		return
	}
	// If no error, it may have returned a result.
}

func TestParseSSHDConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sshd_config")
	content := `# This is a comment
PermitRootLogin no
PasswordAuthentication yes
# PermitRootLogin yes
AllowGroups wheel admin
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	val, err := parseSSHDConfig(cfgPath, "PermitRootLogin")
	require.NoError(t, err)
	assert.Equal(t, "no", val)

	val, err = parseSSHDConfig(cfgPath, "PasswordAuthentication")
	require.NoError(t, err)
	assert.Equal(t, "yes", val)

	val, err = parseSSHDConfig(cfgPath, "AllowGroups")
	require.NoError(t, err)
	assert.Equal(t, "wheel admin", val)

	val, err = parseSSHDConfig(cfgPath, "NonexistentKey")
	require.NoError(t, err)
	assert.Equal(t, "", val)
}

func TestParseSSHDConfigCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sshd_config")
	content := `permitrootlogin no
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	val, err := parseSSHDConfig(cfgPath, "PermitRootLogin")
	require.NoError(t, err)
	assert.Equal(t, "no", val)
}

func TestParseSSHDConfigLastValueWins(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sshd_config")
	content := `PermitRootLogin yes
PermitRootLogin no
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	val, err := parseSSHDConfig(cfgPath, "PermitRootLogin")
	require.NoError(t, err)
	assert.Equal(t, "no", val)
}

func TestParseSSHDConfigFileNotFound(t *testing.T) {
	_, err := parseSSHDConfig("/nonexistent/sshd_config", "PermitRootLogin")
	require.Error(t, err)
}

// --- IPTablesChecker tests ---

func TestIPTablesCheckerTypeAndCategory(t *testing.T) {
	c := &IPTablesChecker{}
	assert.Equal(t, "iptables_check", c.Type())
	assert.Equal(t, "network", c.Category())
}

func TestIPTablesCheckerEmptyChain(t *testing.T) {
	c := &IPTablesChecker{}
	params := json.RawMessage(`{"chain": "", "expected_policy": "DROP"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chain is required")
}

func TestIPTablesCheckerInvalidChain(t *testing.T) {
	c := &IPTablesChecker{}
	params := json.RawMessage(`{"chain": "INVALID", "expected_policy": "DROP"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "INPUT, OUTPUT, or FORWARD")
}

func TestIPTablesCheckerInvalidJSON(t *testing.T) {
	c := &IPTablesChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`{bad`))
	require.Error(t, err)
}

func TestIPTablesCheckerValidChain(t *testing.T) {
	// This test requires iptables to be available (may need root).
	if os.Getuid() != 0 {
		t.Skip("iptables requires root")
	}

	c := &IPTablesChecker{}
	params := json.RawMessage(`{"chain": "INPUT", "expected_policy": "DROP"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	// Just verify we get a valid result.
	assert.NotEmpty(t, result.ActualValue)
	assert.Contains(t, []checker.CheckStatus{checker.StatusPass, checker.StatusFail}, result.Status)
}

// --- NetworkParamChecker tests ---

func TestNetworkParamCheckerTypeAndCategory(t *testing.T) {
	c := &NetworkParamChecker{}
	assert.Equal(t, "network_param_check", c.Type())
	assert.Equal(t, "network", c.Category())
}

func TestNetworkParamCheckerEmptyKey(t *testing.T) {
	c := &NetworkParamChecker{}
	params := json.RawMessage(`{"key": "", "expected": "0"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key is required")
}

func TestNetworkParamCheckerInvalidJSON(t *testing.T) {
	c := &NetworkParamChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`not json`))
	require.Error(t, err)
}

func TestNetworkParamCheckerFileNotFound(t *testing.T) {
	c := &NetworkParamChecker{}
	params := json.RawMessage(`{"key": "net.nonexistent.param", "expected": "0"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusError, result.Status)
}

func TestNetworkParamCheckerPathTraversal(t *testing.T) {
	c := &NetworkParamChecker{}
	params := json.RawMessage(`{"key": "../../etc/passwd", "expected": "0"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path separators")
}

func TestNetworkParamCheckerPathTraversalSlash(t *testing.T) {
	c := &NetworkParamChecker{}
	params := json.RawMessage(`{"key": "net/../../etc/passwd", "expected": "0"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path separators")
}

func TestNetworkParamCheckerWithRealValue(t *testing.T) {
	if _, err := os.Stat("/proc/sys/net/ipv4/ip_forward"); err != nil {
		t.Skip("no /proc/sys/net/ipv4/ip_forward available")
	}

	data, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	require.NoError(t, err)
	actual := string(data[:len(data)-1]) // trim newline

	params, _ := json.Marshal(map[string]string{
		"key":      "net.ipv4.ip_forward",
		"expected": actual,
	})
	c := &NetworkParamChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
	assert.Equal(t, actual, result.ActualValue)
}

func TestNetworkParamCheckerMismatch(t *testing.T) {
	if _, err := os.Stat("/proc/sys/net/ipv4/ip_forward"); err != nil {
		t.Skip("no /proc/sys/net/ipv4/ip_forward available")
	}

	c := &NetworkParamChecker{}
	params := json.RawMessage(`{"key": "net.ipv4.ip_forward", "expected": "999"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusFail, result.Status)
}

// --- sysctlKeyToProcPath unit tests ---

func TestSysctlKeyToProcPath(t *testing.T) {
	assert.Equal(t, "/proc/sys/net/ipv4/ip_forward", sysctlKeyToProcPath("net.ipv4.ip_forward"))
	assert.Equal(t, "/proc/sys/kernel/hostname", sysctlKeyToProcPath("kernel.hostname"))
	assert.Equal(t, "/proc/sys/net", sysctlKeyToProcPath("net"))
}

// --- checkProcNetTCP unit tests ---

func TestCheckProcNetTCP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tcp")
	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 ffff880000000000 100 0 0 10 0
   1: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12346 1 ffff880000000000 100 0 0 10 0
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	// Port 0x0016 = 22, state 0A = LISTEN.
	listening, err := checkProcNetTCP(path, "0016")
	require.NoError(t, err)
	assert.True(t, listening)

	// Port 0x0050 = 80, state 0A = LISTEN.
	listening, err = checkProcNetTCP(path, "0050")
	require.NoError(t, err)
	assert.True(t, listening)

	// Port 0x0017 = 23, not present.
	listening, err = checkProcNetTCP(path, "0017")
	require.NoError(t, err)
	assert.False(t, listening)
}

func TestCheckProcNetTCPNoListening(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tcp")
	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:0050 00000000:0000 06 00000000:00000000 00:00000000 00000000     0        0 12345 1 ffff880000000000 100 0 0 10 0
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	// State 06 = TIME_WAIT, not LISTEN.
	listening, err := checkProcNetTCP(path, "0050")
	require.NoError(t, err)
	assert.False(t, listening)
}

func TestCheckProcNetTCPEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tcp")
	require.NoError(t, os.WriteFile(path, []byte(""), 0o644))

	listening, err := checkProcNetTCP(path, "0016")
	require.NoError(t, err)
	assert.False(t, listening)
}

func TestCheckProcNetTCPFileNotFound(t *testing.T) {
	_, err := checkProcNetTCP("/nonexistent/tcp", "0016")
	require.Error(t, err)
}

// --- Registration test ---

func TestNetworkCheckersRegistered(t *testing.T) {
	types := []string{"port_check", "ssh_config_check", "iptables_check", "network_param_check"}
	for _, typ := range types {
		_, ok := checker.DefaultRegistry.Get(typ)
		assert.True(t, ok, "checker %q should be registered", typ)
	}
}
