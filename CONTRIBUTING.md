# Contributing to OpsAgent

Thank you for your interest in contributing to OpsAgent! This guide will help you set up your development environment and understand our workflow.

## Development Environment

### Prerequisites

- **Go 1.26+** — Primary language for the agent
- **Rust toolchain** (stable) — Required for the plugin runtime (`rust-runtime/`)
- **golangci-lint** — Linter for Go code (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`)
- **gosec** — Security scanner for Go (`go install github.com/securego/gosec/v2/cmd/gosec@latest`)
- **protoc** — Protocol buffer compiler (only if modifying proto files)

### Optional (for sandbox tests)

- **nsjail** — Sandbox execution engine
- **cgroup v2** — Linux kernel feature for resource isolation

You can check sandbox prerequisites with:

```bash
make sandbox-check
```

## Getting Started

1. **Fork and clone** the repository:

   ```bash
   gh repo fork ops-pilot/opsagent --clone
   cd opsagent
   ```

2. **Install dependencies**:

   ```bash
   make tidy
   ```

3. **Run the test suite** to verify your environment:

   ```bash
   make test-race
   ```

4. **Build the project**:

   ```bash
   make build
   ```

## Code Standards

### Formatting

All Go code must be formatted with `gofmt` and `goimports`. CI enforces this — unformatted code will be rejected.

```bash
gofmt -w .
goimports -w .
```

### Style Rules

- **Functions**: Keep under 50 lines. If a function grows beyond that, extract helper functions.
- **Files**: Keep under 800 lines. Split large files into cohesive modules.
- **Packages**: Single responsibility, high cohesion, low coupling.
- **`main.go`**: No business logic. The entry point only handles startup.
- **No unnecessary dependencies**: Avoid adding third-party libraries unless there is a clear, justified need.

### Error Handling

Wrap errors with context using `fmt.Errorf`:

```go
result, err := doSomething()
if err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}
```

All errors must be logged and returned up the call stack. Never silently swallow errors.

### Interfaces

Define interfaces at the **usage** site, not the implementation site. This keeps dependencies inverted and makes testing easier:

```go
// In the consumer package (e.g., internal/app):
type Collector interface {
    Collect(ctx context.Context) (map[string]float64, error)
}

// In the implementation package (e.g., internal/collector):
type HostCollector struct { ... }
```

### Immutability

Prefer immutable patterns. Create new objects rather than mutating existing ones. This prevents hidden side effects and makes concurrent code safer.

## Testing Requirements

### Coverage

CI enforces a **minimum 80% test coverage**. Generated protobuf code is excluded from the coverage calculation.

### Test Types

| Type | Command | Description |
|------|---------|-------------|
| Unit tests | `make test` | Basic test run |
| Race detector | `make test-race` | Tests with `-race` flag (required) |
| Coverage report | `make test-cover` | Generates `coverage.out` |
| Integration tests | `make integration` | Tests against real services |
| Sandbox tests | `make integration-sandbox` | Tests with nsjail (requires root) |
| E2E tests | `make e2e` | End-to-end tests with `e2e` build tag |
| Benchmarks | `make bench` | Performance benchmarks for collector |
| Full CI pipeline | `make ci` | tidy, vet, test-race, security |

### Writing Tests

Use **table-driven tests** with [testify](https://github.com/stretchr/testify):

```go
func TestParseConfig(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    *Config
        wantErr bool
    }{
        {name: "valid config", input: "port: 8080", want: &Config{Port: 8080}, wantErr: false},
        {name: "invalid port", input: "port: -1", want: nil, wantErr: true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseConfig(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            assert.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### Race Detector

All tests must pass with the race detector enabled. Run `make test-race` before submitting.

## Commit Convention

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>: <description>
```

| Type | Use for |
|------|---------|
| `feat` | New features |
| `fix` | Bug fixes |
| `refactor` | Code restructuring without behavior change |
| `docs` | Documentation changes |
| `test` | Adding or updating tests |
| `chore` | Build, CI, tooling changes |
| `perf` | Performance improvements |
| `ci` | CI/CD pipeline changes |

Examples:

```
feat: add cgroup v2 resource limits
fix: prevent nil pointer in collector shutdown
refactor: extract sandbox config into separate file
```

## Pull Request Process

1. **Create a feature branch** from `main`:

   ```bash
   git checkout -b feat/my-feature main
   ```

2. **Write tests first** (TDD encouraged). See [Testing Requirements](#testing-requirements).

3. **Implement your changes**, keeping commits focused and well-described.

4. **Run the full CI pipeline** locally:

   ```bash
   make ci
   ```

5. **Push and open a pull request** against `main`.

6. **Request review** from maintainers.

7. **Squash merge** after approval. Maintainers will handle the merge.

### PR Checklist

Before requesting review, verify:

- [ ] `make ci` passes locally
- [ ] Test coverage is at or above 80%
- [ ] Code is formatted with `gofmt` and `goimports`
- [ ] Errors are wrapped with context
- [ ] No hardcoded secrets or credentials
- [ ] Documentation is updated (if applicable)

## Security Requirements

OpsAgent executes commands on host machines, so security is critical.

### Mandatory Rules

- **No hardcoded secrets**: Use environment variables or configuration files. Never commit API keys, passwords, or tokens.
- **Command execution through whitelist only**: All commands must pass through the executor's whitelist. Never bypass it by calling `exec.Command` directly.
- **Use `context` for timeouts**: All I/O operations must respect context cancellation and deadlines.
- **Limit output size**: stdout/stderr buffers must have size limits to prevent memory exhaustion.
- **No shell strings**: Never execute raw shell strings or use `sh -c`. Commands must be invoked as structured arguments.
- **Plugin tasks via UDS RPC**: Plugin work must go through the local Unix Domain Socket RPC interface, not direct system calls.

### Security Scanning

Run `gosec` before submitting:

```bash
make security
```

CI will also run `cargo audit` for the Rust runtime to catch known vulnerabilities.

## Documentation

- **Update docs** when adding or changing features. Documentation lives in the `docs/` directory.
- **Keep zh/en in sync**: If documentation exists in both Chinese and English, update both versions.
- **Code comments**: Explain *why*, not *what*. The code itself should be readable enough to show *what*.

## Project Structure

```
cmd/agent/              — Program entry point (startup only)
internal/app/           — Lifecycle orchestration and dependency wiring
internal/config/        — Configuration model, defaults, validation
internal/collector/     — Metric collection interfaces and implementations
internal/executor/      — Command execution and whitelist security boundary
internal/server/        — HTTP API, auth middleware, Prometheus export
internal/task/          — Task model and dispatcher
internal/reporter/      — stdout/http reporting strategies
internal/pluginruntime/ — Rust runtime process management and UDS RPC client
internal/health/        — Health check interface (Statuser)
internal/sandbox/       — nsjail sandbox engine, seccomp policy, audit logging
rust-runtime/           — Rust plugin runtime implementation
```

## Getting Help

- Open an issue for bugs or feature requests
- Start a discussion for questions or design proposals
- Refer to `AGENTS.md` for detailed architecture guidance
