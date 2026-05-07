package network

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&PortChecker{})
}

// PortChecker checks whether a TCP port is listening by parsing /proc/net/tcp and /proc/net/tcp6.
type PortChecker struct{}

func (c *PortChecker) Type() string     { return "port_check" }
func (c *PortChecker) Category() string { return "network" }

type portParams struct {
	Port         int    `json:"port"`
	ExpectedState string `json:"expected_state"` // "listening" or "not_listening"
}

func (c *PortChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p portParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("port_check: invalid params: %w", err)
	}

	if p.Port <= 0 || p.Port > 65535 {
		return nil, fmt.Errorf("port_check: port must be 1-65535, got %d", p.Port)
	}

	if p.ExpectedState != "listening" && p.ExpectedState != "not_listening" {
		return nil, fmt.Errorf("port_check: expected_state must be 'listening' or 'not_listening', got %q", p.ExpectedState)
	}

	start := time.Now()

	listening, err := isPortListening(p.Port)
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to check port %d: %v", p.Port, err),
			Duration: time.Since(start),
		}, nil
	}

	actualState := "not_listening"
	if listening {
		actualState = "listening"
	}

	result := &checker.CheckResult{
		ActualValue:   actualState,
		ExpectedValue: p.ExpectedState,
		Duration:      time.Since(start),
	}

	if actualState == p.ExpectedState {
		result.Status = checker.StatusPass
		result.Message = fmt.Sprintf("port %d is %s (expected)", p.Port, actualState)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("port %d is %s, expected %s", p.Port, actualState, p.ExpectedState)
	}

	return result, nil
}

// isPortListening checks /proc/net/tcp and /proc/net/tcp6 for a listening port.
// In /proc/net/tcp, state "0A" means LISTEN.
func isPortListening(port int) (bool, error) {
	hexPort := fmt.Sprintf("%04X", port)

	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		listening, err := checkProcNetTCP(path, hexPort)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return false, err
		}
		if listening {
			return true, nil
		}
	}
	return false, nil
}

// checkProcNetTCP parses a /proc/net/tcp-style file looking for a listening port.
func checkProcNetTCP(path, hexPort string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Skip header line.
	if scanner.Scan() {
		// header consumed
	}

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		// fields[0]: sl (index), fields[1]: local_address, fields[3]: st (state)
		localAddr := fields[1]
		state := fields[3]

		// local_address format is HEXIP:HEXPORT
		parts := strings.SplitN(localAddr, ":", 2)
		if len(parts) != 2 {
			continue
		}

		if parts[1] == hexPort && state == "0A" {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, nil
}

