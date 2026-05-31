package syslog

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

func init() {
	collector.RegisterInput("syslog", func() collector.Input {
		return &SyslogInput{}
	})
}

// SyslogInput listens for syslog messages over TCP or UDP
// and emits them as metrics.
type SyslogInput struct {
	ListenAddr     string `toml:"listen_addr"`
	Protocol       string `toml:"protocol"`
	MaxConnections int    `toml:"max_connections"`

	listener    net.Listener
	udpConn     *net.UDPConn
	ready       chan struct{} // closed when listener is ready; created per Gather call
	parseErrors int64         // atomic counter for parse failures
}

// Init parses the config map and sets defaults.
func (s *SyslogInput) Init(cfg map[string]any) error {
	s.ListenAddr = "0.0.0.0:514"
	s.Protocol = "tcp"
	s.MaxConnections = 100
	s.ready = make(chan struct{})

	if v, ok := cfg["listen_addr"]; ok {
		addr, ok := v.(string)
		if !ok {
			return fmt.Errorf("syslog: listen_addr must be a string, got %T", v)
		}
		s.ListenAddr = addr
	}
	if v, ok := cfg["protocol"]; ok {
		proto, ok := v.(string)
		if !ok {
			return fmt.Errorf("syslog: protocol must be a string, got %T", v)
		}
		if proto != "tcp" && proto != "udp" {
			return fmt.Errorf("syslog: unsupported protocol %q (supported: tcp, udp)", proto)
		}
		s.Protocol = proto
	}
	if v, ok := cfg["max_connections"]; ok {
		switch n := v.(type) {
		case int:
			s.MaxConnections = n
		case int64:
			s.MaxConnections = int(n)
		case float64:
			s.MaxConnections = int(n)
		default:
			return fmt.Errorf("syslog: max_connections must be a number, got %T", v)
		}
	}
	return nil
}

// Gather listens for syslog messages and emits them as metrics.
// It blocks until the context is cancelled.
func (s *SyslogInput) Gather(ctx context.Context, acc collector.Accumulator) error {
	switch s.Protocol {
	case "tcp":
		return s.gatherTCP(ctx, acc)
	case "udp":
		return s.gatherUDP(ctx, acc)
	default:
		return fmt.Errorf("syslog: unsupported protocol %q", s.Protocol)
	}
}

// SampleConfig returns a sample configuration string.
func (s *SyslogInput) SampleConfig() string {
	return `
  ## Address to listen on for syslog messages
  # listen_addr = "0.0.0.0:514"
  ## Protocol: tcp or udp
  # protocol = "tcp"
  ## Maximum concurrent TCP connections
  # max_connections = 100
`
}

// gatherTCP listens on a TCP socket and processes line-delimited syslog messages.
func (s *SyslogInput) gatherTCP(ctx context.Context, acc collector.Accumulator) error {
	ln, err := net.Listen("tcp", s.ListenAddr)
	if err != nil {
		return fmt.Errorf("syslog: listen tcp: %w", err)
	}
	s.listener = ln
	close(s.ready)

	// Close listener when context is cancelled
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	sem := make(chan struct{}, s.MaxConnections)
	var wg sync.WaitGroup

	for {
		conn, err := ln.Accept()
		if err != nil {
			// Check if context was cancelled
			if ctx.Err() != nil {
				break
			}
			// Brief backoff to avoid busy-loop on transient errors (e.g. EMFILE/ENFILE)
			time.Sleep(10 * time.Millisecond)
			continue
		}

		wg.Go(func() {
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			// Non-EOF scanner errors are already surfaced via handleTCPConn's
			// return value.  We intentionally do not propagate them further
			// because each connection runs in its own goroutine and a single
			// bad connection must not abort the whole listener.
			_ = s.handleTCPConn(conn, acc)
		})
	}

	wg.Wait()
	return ctx.Err()
}

// handleTCPConn reads line-delimited syslog messages from a TCP connection.
// It returns scanner.Err() after the read loop completes.
func (s *SyslogInput) handleTCPConn(conn net.Conn, acc collector.Accumulator) error {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		s.processMessage(line, acc)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// gatherUDP listens on a UDP socket and processes syslog datagrams.
func (s *SyslogInput) gatherUDP(ctx context.Context, acc collector.Accumulator) error {
	addr, err := net.ResolveUDPAddr("udp", s.ListenAddr)
	if err != nil {
		return fmt.Errorf("syslog: resolve udp: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("syslog: listen udp: %w", err)
	}
	s.udpConn = conn
	close(s.ready)

	// Close connection when context is cancelled
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	buf := make([]byte, 65536)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			s.processMessage(data, acc)
		}
	}
}

// processMessage parses a syslog message and emits it as a metric.
func (s *SyslogInput) processMessage(data []byte, acc collector.Accumulator) {
	msg, err := Parse(data)
	if err != nil {
		atomic.AddInt64(&s.parseErrors, 1)
		return
	}

	tags := map[string]string{
		"app":  msg.AppName,
		"host": msg.Hostname,
	}
	fields := map[string]any{
		"message":  msg.Message,
		"facility": msg.Facility,
		"severity": msg.Severity,
		"pid":      msg.ProcID,
	}

	if !msg.Timestamp.IsZero() {
		acc.AddGaugeWithTimestamp("syslog", tags, fields, msg.Timestamp)
	} else {
		acc.AddGauge("syslog", tags, fields)
	}
}
