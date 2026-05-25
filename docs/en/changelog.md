# OpsAgent Changelog

This document records the version change history of the OpsAgent project, listed in reverse chronological order.

---

## 2026-05-01 -- Security Hardening Sprint

This update implements comprehensive security hardening across multiple OpsAgent subsystems, covering authentication and authorization, input validation, sandbox isolation, network communication, and more.

### Global Security Hardening

- Implemented comprehensive security hardening measures across multiple subsystems

### Authentication and Authorization

- Authentication enabled by default, restricted to localhost only, token minimum length requirement
- Bearer token validation using constant-time comparison to prevent timing attacks
- Prevented disabling authentication via hot reload mechanism

### Input Validation and Request Security

- Request body size limited to 1MB on exec and task endpoints
- Null byte and length validation in command arguments
- TaskID sanitization to prevent path traversal in sandbox file operations
- Extended TaskID sanitization to all entry points, improved test coverage
- Sanitized error messages in API responses to prevent information leakage

### Sandbox Security

- Reject unknown interpreters, no longer pass raw input
- Added missing shell metacharacters to sandbox policy
- Use unpredictable temporary paths, audit log permissions restricted to 0600
- Removed /etc mount, added resource limits, check metacharacters in command names
- Made seccomp policy dynamic, removed clone/fork/vfork to prevent fork bombs
- Sanitized environment variables passed to sandbox, blocked LD_PRELOAD
- Prevented plugin binary path traversal, sanitized socket paths, blocked symlink attacks
- Sanitized plugin environment variables, socket permissions set to 0600

### Network and Communication Security

- Validate IP addresses before passing to iptables
- Reject insecure gRPC connections, enforce TLS 1.2+, set ServerName
- Validate HTTP output URL protocol scheme, hide version info from unauthenticated health endpoints

### HTTP Server Security

- Added security response headers, restricted HTTP methods, timeout capped at 300 seconds
- Added IP-based rate limiting middleware (10 requests/second, burst 20)

### Configuration and File Security

- Mask sensitive information in config diffs, file permissions set to 0600, ConfigUpdate size limit
- Escape Prometheus label values, default binding to 127.0.0.1:9100

### System Service Security

- Set NoNewPrivileges=true in systemd service unit
- Added FsScan path whitelist, limited dmesg output, require socket environment variable

### Tests

- Added comprehensive tests for HTTP and stdout reporters, sandbox executor, and server-side handlers
- Fixed request body size limit tests, ensured regression test effectiveness

---

## 2026-04-30 -- SDK and Code Review

This update introduces the Plugin SDK (Go and Rust), metrics system, audit logging, health checks, and other core infrastructure, along with comprehensive code review improvements.

### Plugin SDK

- Added Go Plugin SDK with Handler interface and Serve function
- Added Rust Plugin SDK with Plugin trait and serve function
- Added Go echo example plugin
- Added Rust audit example plugin

### Plugin Gateway and Runtime

- Added PluginGateway interface and PluginInfo type
- Added PluginGatewayConfig with default values and validation logic
- Integrated PluginGateway into Agent lifecycle
- Added PluginManifest type, parsing, and validation
- Added plugin manifest file watcher with debouncing
- Added end-to-end integration tests for PluginGateway

### Metrics System

- Replaced hand-written Prometheus metrics with client_golang registry
- Added IncPipelineErrors and IncPluginRequests convenience methods
- Integrated IncMetricsCollected and IncPluginRequests counters into Agent

### Audit Logging

- Added structured JSON-lines audit logger with log rotation
- Added gRPC connect/disconnect audit events
- Added config, plugin, task cancel, and sandbox audit events

### Health Check

- Added HealthStatus for subsystems, enhanced /healthz endpoint
- Added last_collection field to health status
- Propagated version info from Agent to health endpoint

### Agent Core

- Integrated metrics, audit, health check, version info, and RunOnce into Agent
- Used totalFields in RunOnce output

### CI/CD and Engineering

- Added 80% coverage gate, Rust CI job, and integration test job
- Updated CI configuration for Go and Rust, added caching and linting
- Added FORCE_JAVASCRIPT_ACTIONS_TO_NODE24 environment variable
- Added Rust dependencies for integration jobs
- Added Prometheus client_golang, lumberjack, testify dependencies, updated Makefile
- Promoted lumberjack to direct dependency
- Added benchmarks for metric collection and pipeline processing

### Documentation and Code Cleanup

- Added plugin contract documentation
- Added code review fix design specification
- Improved code readability and organization in multiple files
- Removed Go example binaries and Rust target directory from Git, updated .gitignore
- Cleaned up testify indirect dependencies in go.sum

---

## 2026-04-29 -- Major Feature Day

Multiple parallel workflows merged, covering Rust runtime handlers, new collection plugins, configuration hot reload, graceful shutdown, and other core features.

### Rust Runtime Handlers

6 new handler implementations:
- **log_parse** -- Log parsing
- **text_process** -- Text processing
- **fs_scan** -- Filesystem scanning
- **ebpf_collect** -- eBPF data collection
- **conn_analyze** -- Connection analysis
- **local_probe** -- Local probing

### Collector New Input Plugins

- **connections** -- Network connection collection
- **load** -- System load collection
- **gpu** -- GPU metric collection
- **temp** -- Temperature sensor collection
- **diskio** -- Disk I/O collection

### Aggregator

- Added percentile aggregator

### Configuration Hot Reload

- Added ConfigReloader with configuration hot reload support
- Implemented atomic rollback mechanism
- Added configuration diff engine

### Application Lifecycle

- Implemented graceful shutdown
- Added SIGHUP signal handler

### gRPC Client

- Added FlushAndStop functionality
- Implemented cache persistence

### Subsystem Reload

- Added Reloader implementation for Collector, Server, Reporter

### Engineering Quality

- Improved test coverage
- Code refactoring
- Agent dependency injection improvements

---

## 2026-04-28 -- Initial Release

OpsAgent project first release, including complete collection pipeline, sandbox execution, gRPC communication, and HTTP server core features.

### Project Infrastructure

- Project initialization commit
- Proto definition and gRPC framework setup
- Cobra-based command line interface (CLI)

### Sandbox Executor

- nsjail configuration and security policy
- Network isolation
- cgroup resource statistics
- Audit logging
- Output streaming
- Sandbox executor core implementation

### Collection Pipeline

- Metric data model definition
- Plugin interface specification
- Plugin registry
- Metric Accumulator
- Metric Buffer
- Collection Scheduler

### Input Plugins

- **cpu** -- CPU usage collection
- **memory** -- Memory usage collection
- **disk** -- Disk usage collection
- **net** -- Network traffic collection
- **process** -- Process information collection

### Processor Plugins

- **tagger** -- Label tagger
- **regex** -- Regular expression processor

### Aggregator Plugins

- **avg** -- Average aggregation
- **sum** -- Sum aggregation

### Output Plugins

- **http** -- HTTP output
- **prometheus** -- Prometheus metrics exposure
- **promrw** -- Prometheus Remote Write output

### gRPC Client

- Data Sender
- Data Receiver
- Data Cache
- Reconnection

### HTTP Server

- HTTP server and handler implementation

### Tests

- Integration test suite
