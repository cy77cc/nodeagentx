package plugin

// TaskRequest is the incoming task dispatched by the PluginGateway.
type TaskRequest struct {
	TaskID   string                 `json:"task_id"`
	TaskType string                 `json:"task_type"`
	Params   map[string]any `json:"params"`
	Deadline int64                  `json:"deadline_ms"`
}

// TaskResponse is returned by the plugin after executing a task.
type TaskResponse struct {
	TaskID string      `json:"task_id"`
	Status string      `json:"status"` // "ok" or "error"
	Data   any `json:"data,omitzero"`
	Error  string      `json:"error,omitzero"`
}

// Chunk represents a piece of a large output, base64-encoded.
type Chunk struct {
	Seq    int    `json:"seq"`
	EOF    bool   `json:"eof"`
	DataB64 string `json:"data_b64"`
}

// TaskStats holds execution statistics for a task.
type TaskStats struct {
	DurationMS    int64 `json:"duration_ms"`
	CPUMS         int64 `json:"cpu_ms"`
	MemPeakBytes  int64 `json:"mem_peak_bytes"`
}

// rpcRequest is the internal JSON-RPC request envelope.
type rpcRequest struct {
	ID     any `json:"id"`
	Method string      `json:"method"`
	Params TaskRequest `json:"params"`
}

// rpcError represents a JSON-RPC error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// rpcResponse is the internal JSON-RPC response envelope.
type rpcResponse struct {
	ID     any `json:"id"`
	Result any `json:"result,omitzero"`
	Error  *rpcError   `json:"error,omitzero"`
}
