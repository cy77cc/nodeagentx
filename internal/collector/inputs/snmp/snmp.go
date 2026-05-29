package snmp

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/cy77cc/opsagent/internal/collector"
)

func init() {
	collector.RegisterInput("snmp", func() collector.Input {
		return &SNMPInput{}
	})
}

// SNMPInput queries SNMP agents and collects metrics.
type SNMPInput struct {
	Agents    []string
	Community string
	Version   int
	OIDs      []string
	Timeout   int
}

// Init parses the config map and sets defaults.
func (s *SNMPInput) Init(cfg map[string]interface{}) error {
	// Set defaults
	s.Version = 2
	s.Community = "public"
	s.Timeout = 5

	if v, ok := cfg["agents"]; ok {
		agents, ok := v.([]interface{})
		if !ok {
			return fmt.Errorf("snmp: agents must be a list, got %T", v)
		}
		s.Agents = make([]string, 0, len(agents))
		for i, a := range agents {
			agent, ok := a.(string)
			if !ok {
				return fmt.Errorf("snmp: agents[%d] must be a string, got %T", i, a)
			}
			s.Agents = append(s.Agents, agent)
		}
	}

	if v, ok := cfg["community"]; ok {
		community, ok := v.(string)
		if !ok {
			return fmt.Errorf("snmp: community must be a string, got %T", v)
		}
		s.Community = community
	}

	if v, ok := cfg["version"]; ok {
		version, ok := toInt(v)
		if !ok {
			return fmt.Errorf("snmp: version must be a number, got %T", v)
		}
		if version != 1 && version != 2 && version != 3 {
			return fmt.Errorf("snmp: unsupported SNMP version %d, must be 1, 2, or 3", version)
		}
		s.Version = version
	}

	if v, ok := cfg["oids"]; ok {
		oids, ok := v.([]interface{})
		if !ok {
			return fmt.Errorf("snmp: oids must be a list, got %T", v)
		}
		s.OIDs = make([]string, 0, len(oids))
		for i, o := range oids {
			oid, ok := o.(string)
			if !ok {
				return fmt.Errorf("snmp: oids[%d] must be a string, got %T", i, o)
			}
			s.OIDs = append(s.OIDs, oid)
		}
	}

	if v, ok := cfg["timeout"]; ok {
		timeout, ok := toInt(v)
		if !ok {
			return fmt.Errorf("snmp: timeout must be a number, got %T", v)
		}
		s.Timeout = timeout
	}

	return nil
}

// Gather connects to each SNMP agent, queries the configured OIDs, and emits metrics.
func (s *SNMPInput) Gather(ctx context.Context, acc collector.Accumulator) error {
	for _, agent := range s.Agents {
		if err := s.gatherAgent(ctx, agent, acc); err != nil {
			return err
		}
	}
	return nil
}

// gatherAgent connects to a single SNMP agent and collects metrics.
func (s *SNMPInput) gatherAgent(ctx context.Context, agent string, acc collector.Accumulator) error {
	// Ensure agent has a port
	host, port, err := net.SplitHostPort(agent)
	if err != nil {
		// No port specified, default to 161
		host = agent
		port = "161"
	}
	agentAddr := net.JoinHostPort(host, port)

	client := &gosnmp.GoSNMP{
		Target:    host,
		Port:      parsePort(port),
		Community: s.Community,
		Version:   snmpVersion(s.Version),
		Timeout:   time.Duration(s.Timeout) * time.Second,
		Retries:   1,
	}

	if err := client.Connect(); err != nil {
		return fmt.Errorf("snmp: connecting to %s: %w", agentAddr, err)
	}
	defer client.Conn.Close()

	// Check context before making request
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	result, err := client.Get(s.OIDs)
	if err != nil {
		return fmt.Errorf("snmp: getting OIDs from %s: %w", agentAddr, err)
	}

	tags := map[string]string{
		"agent": agentAddr,
	}
	fields := make(map[string]interface{})

	for _, pdu := range result.Variables {
		value := snmpValue(pdu)
		if value != nil {
			fields[pdu.Name] = value
		}
	}

	if len(fields) > 0 {
		acc.AddFields("snmp", tags, fields)
	}

	return nil
}

// SampleConfig returns a sample configuration for the SNMP input.
func (s *SNMPInput) SampleConfig() string {
	return `
  ## List of SNMP agents to query
  # agents = ["127.0.0.1:161"]

  ## SNMP community string
  # community = "public"

  ## SNMP version (1, 2, or 3)
  # version = 2

  ## List of OIDs to query
  # oids = ["1.3.6.1.2.1.1.3.0"]

  ## Timeout in seconds
  # timeout = 5
`
}

// snmpValue converts a gosnmp PDU value to a Go type suitable for metric fields.
func snmpValue(pdu gosnmp.SnmpPDU) interface{} {
	switch pdu.Type {
	case gosnmp.OctetString:
		return string(pdu.Value.([]byte))
	case gosnmp.Integer:
		return pdu.Value.(int)
	case gosnmp.Counter32:
		return pdu.Value.(uint)
	case gosnmp.Gauge32:
		return pdu.Value.(uint)
	case gosnmp.TimeTicks:
		return pdu.Value.(uint32)
	case gosnmp.Counter64:
		return pdu.Value.(uint64)
	case gosnmp.Uinteger32:
		return pdu.Value.(uint32)
	case gosnmp.ObjectIdentifier:
		return pdu.Value.(string)
	default:
		return pdu.Value
	}
}

// snmpVersion converts an int SNMP version to gosnmp version constant.
func snmpVersion(v int) gosnmp.SnmpVersion {
	switch v {
	case 1:
		return gosnmp.Version1
	case 2:
		return gosnmp.Version2c
	case 3:
		return gosnmp.Version3
	default:
		return gosnmp.Version2c
	}
}

// toInt converts interface{} values to int, handling both int and float64 (from JSON).
func toInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	default:
		return 0, false
	}
}

// parsePort parses a port string to uint16, defaulting to 161.
func parsePort(port string) uint16 {
	var p uint16
	_, err := fmt.Sscanf(port, "%d", &p)
	if err != nil || p == 0 {
		return 161
	}
	return p
}
