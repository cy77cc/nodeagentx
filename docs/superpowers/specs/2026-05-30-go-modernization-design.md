# Go 1.26 项目现代化设计文档

## 概述

将 opsagent 项目中过时的 Go 模式和语法升级为 Go 1.26 最新惯用写法。

- **范围：** 全部 284 个 `.go` 文件（生产代码 + 测试代码）
- **策略：** `go fix ./...` 自动化 + 手动补充
- **Go 版本：** 已使用 go 1.26.1，所有新特性可用
- **不涉及：** 生成的 protobuf 文件 (`*.pb.go`)

## 变更分类

### Phase 1: `go fix` 自动化

运行 `go fix ./...`，让 Go 官方 fixer 自动处理它能识别的模式。

```bash
go fix ./...
go build ./...
go vet ./...
```

### Phase 2: Stdlib 替换

#### 2a. `sort.Strings` → `slices.Sort` (6 处)

| 文件 | 行号 |
|------|------|
| `internal/collector/registry_test.go` | 58 |
| `internal/collector/processors/delta/delta.go` | 34 |
| `internal/collector/outputs/prometheus/prometheus.go` | 126, 151 |
| `internal/collector/outputs/promrw/promrw.go` | 113 |
| `internal/federation/group.go` | 107 |

同时将 `import "sort"` 替换/合并为 `import "slices"`（如果该文件不再使用 `sort` 包的其他函数）。

#### 2b. `sort.Slice` → `slices.SortFunc` (1 处)

| 文件 | 行号 |
|------|------|
| `internal/collector/inputs/process/process.go` | 113 |

将 `sort.Slice(infos, func(i, j int) bool { ... })` 改为 `slices.SortFunc(infos, func(a, b ProcessInfo) int { return cmp.Compare(a.PID, b.PID) })`（或对应的比较字段）。

#### 2c. 自定义 `contains` 函数 → `slices.Contains` (2 个函数)

| 文件 | 函数 |
|------|------|
| `internal/discovery/proc.go:131-138` | `containsPort(ports []int, port int) bool` |
| `internal/federation/group.go:148-155` | `contains(slice []string, val string) bool` |

删除自定义函数，替换所有调用处为 `slices.Contains(...)`。

#### 2d. Map keys 收集 + sort → `slices.Sorted(maps.Keys(...))` (7-10 处)

将以下模式：
```go
keys := make([]string, 0, len(m))
for k := range m {
    keys = append(keys, k)
}
slices.Sort(keys) // 或 sort.Strings(keys)
```

替换为：
```go
keys := slices.Sorted(maps.Keys(m))
```

涉及文件：
- `internal/collector/processors/delta/delta.go:31-34`
- `internal/collector/outputs/prometheus/prometheus.go:123-126, 148-151`
- `internal/collector/outputs/promrw/promrw.go:110-113`
- `internal/collector/registry.go:119-122`
- `internal/checker/registry.go:39-42`
- `internal/federation/group.go:104-107`

### Phase 3: 并发和测试现代化

#### 3a. `wg.Add(1)` + goroutine + `defer wg.Done()` → `wg.Go(func() { ... })` (11 个文件)

Go 1.24 引入的 `sync.WaitGroup.Go()` 方法将 goroutine 启动和计数合并为一步。

将：
```go
wg.Add(1)
go func() {
    defer wg.Done()
    // ...
}()
```

替换为：
```go
wg.Go(func() {
    // ...
})
```

涉及文件：
- `internal/collector/scheduler.go`
- `internal/collector/inputs/syslog/syslog.go`
- `internal/pluginruntime/gateway.go`
- `internal/pluginruntime/watcher.go`
- `internal/grpcclient/client.go`
- `internal/gateway/gateway.go`
- `sdk/plugin/serve.go`
- 测试文件：`agent_test.go`, `minmax_test.go`, `percentile_test.go`, `delta_test.go`

**注意：** 需要检查每个 `wg.Add(1)` 的位置——有些可能在循环外批量 Add，然后在多个 goroutine 中 Done。这种模式不能直接用 `wg.Go()` 替换，需要重构为每个 goroutine 调用 `wg.Go()`。

#### 3b. `for i := 0; i < b.N; i++` → `for range b.N` 或 `b.Loop()` (2 处)

| 文件 | 行号 |
|------|------|
| `internal/collector/benchmark_test.go` | 70, 87 |

Go 1.24 引入 `testing.B.Loop()`，Go 1.22 引入 range-over-int。推荐用 `for range b.N`（更简洁）或 `b.Loop()`（更准确，避免编译器优化问题）。

#### 3c. `errors.As` → `errors.AsType[T]` (2 处)

| 文件 | 行号 |
|------|------|
| `internal/sandbox/executor.go` | 271 |
| `internal/executor/executor.go` | 115 |

将：
```go
var exitErr *exec.ExitError
if errors.As(err, &exitErr) {
```

替换为：
```go
if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
```

### Phase 4: 语法糖和 JSON 标签

#### 4a. `for i := 0; i < N; i++` → `for i := range N` (~17 处简单计数循环)

仅替换步长为 1 的简单循环。非步长为 1 的（如 `i += batchSize`）不动。

涉及文件：
- `internal/collector/buffer.go:61`
- `internal/collector/inputs/process/process.go:122`
- `internal/pluginruntime/gateway.go:620`
- `internal/federation/config_distributor.go:45`
- `internal/grpcclient/cache.go:57`
- 测试文件：`buffer_test.go`, `benchmark_test.go`, `agent_test.go`, `percentile_test.go`, `minmax_test.go`, `delta_test.go`, `syslog_test.go`, `server_test.go`, `output_streamer_test.go`, `client_test.go`

#### 4b. `omitempty` → `omitzero` (~20 处手写 struct tag)

将所有手写 struct 的 `json:"...,omitempty"` 替换为 `json:"...,omitzero"`。

涉及文件（排除 `*.pb.go`）：
- `internal/app/audit.go`
- `internal/health/health.go`
- `internal/checker/filesystem/dir_perm.go`
- `internal/discovery/discovery.go`
- `internal/server/handlers.go`
- `internal/pluginruntime/types.go`
- `internal/sandbox/audit.go`
- `internal/sandbox/executor.go`
- `internal/federation/types.go`
- `internal/federation/operation.go`
- `sdk/plugin/protocol.go`

## 不做的事情

以下模式**不改**：
- **批量步长循环** (`i += batchSize`)：`for i := range n` 不支持非单位步长，保持原样
- **生成的 protobuf 文件** (`*.pb.go`)：不应该手动修改
- **`errors.Is` 用法**：已经是惯用写法，无需改动
- **条件性 delete 循环**：selective delete 不适合用 `clear()` 替代

## 验证策略

每个 Phase 完成后：
```bash
go build ./...     # 编译通过
go vet ./...       # 静态检查通过
go test ./...      # 测试通过
```

Phase 4b（omitzero）完成后额外验证 JSON 序列化行为是否符合预期。

## 预期变更量

- ~80-100 处代码变更
- 涉及 ~30-40 个文件
- 纯语法/API 升级，无功能变更
