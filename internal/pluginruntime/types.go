package pluginruntime

// ChunkingConfig controls chunked response behavior.
type ChunkingConfig struct {
	Enabled       bool `json:"enabled"`
	MaxChunkBytes int  `json:"max_chunk_bytes"`
	MaxTotalBytes int  `json:"max_total_bytes"`
}

// TaskRequest is the runtime request payload.
type TaskRequest struct {
	TaskID     string         `json:"task_id"`
	Type       string         `json:"type"`
	DeadlineMS int64          `json:"deadline_ms"`
	Payload    map[string]any `json:"payload"`
	Chunking   ChunkingConfig `json:"chunking"`
}

// Chunk carries a slice of large task output.
type Chunk struct {
	Seq     int    `json:"seq"`
	EOF     bool   `json:"eof"`
	DataB64 string `json:"data_b64"`
}

// TaskStats contains runtime execution stats.
type TaskStats struct {
	DurationMS   int64 `json:"duration_ms"`
	CPUMS        int64 `json:"cpu_ms,omitzero"`
	MemPeakBytes int64 `json:"mem_peak_bytes,omitzero"`
}

// TaskResponse is the runtime response payload.
type TaskResponse struct {
	TaskID  string         `json:"task_id"`
	Status  string         `json:"status"`
	Error   string         `json:"error,omitzero"`
	Summary map[string]any `json:"summary,omitzero"`
	Chunks  []Chunk        `json:"chunks,omitzero"`
	Stats   TaskStats      `json:"stats"`
}

type rpcRequest struct {
	ID     any         `json:"id"`
	Method string      `json:"method"`
	Params TaskRequest `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	ID     any           `json:"id"`
	Result *TaskResponse `json:"result,omitzero"`
	Error  *rpcError     `json:"error,omitzero"`
}
