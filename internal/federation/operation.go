package federation

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

const (
	OpStatusPending        = "pending"
	OpStatusRunning        = "running"
	OpStatusCompleted      = "completed"
	OpStatusPartialFailure = "partial_failure"
	OpStatusFailed         = "failed"
	OpStatusRolledBack     = "rolled_back"
)

type Operation struct {
	ID          string                   `json:"id"`
	Type        string                   `json:"type"`
	TargetGroup string                   `json:"target_group"`
	Params      map[string]string        `json:"params"`
	Status      string                   `json:"status"`
	LeafResults map[string]*LeafOpResult `json:"leaf_results"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
}

type LeafOpResult struct {
	Status     string    `json:"status"`
	Error      string    `json:"error,omitzero"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

type OperationStatus struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	TargetGroup string `json:"target_group"`
	Status      string `json:"status"`
	Total       int    `json:"total"`
	Success     int    `json:"success"`
	Failed      int    `json:"failed"`
	Pending     int    `json:"pending"`
}

type OperationManager struct {
	mu         sync.RWMutex
	operations map[string]*Operation
}

func NewOperationManager() *OperationManager {
	return &OperationManager{
		operations: make(map[string]*Operation),
	}
}

func (om *OperationManager) Create(opType, targetGroup string, params map[string]string) (*Operation, error) {
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("generate operation id: %w", err)
	}
	op := &Operation{
		ID:          id,
		Type:        opType,
		TargetGroup: targetGroup,
		Params:      params,
		Status:      OpStatusPending,
		LeafResults: make(map[string]*LeafOpResult),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	om.mu.Lock()
	om.operations[id] = op
	om.mu.Unlock()
	return op, nil
}

func (om *OperationManager) GetStatus(opID string) (*OperationStatus, error) {
	om.mu.RLock()
	defer om.mu.RUnlock()
	op, ok := om.operations[opID]
	if !ok {
		return nil, fmt.Errorf("operation %s not found", opID)
	}
	status := &OperationStatus{
		ID:          op.ID,
		Type:        op.Type,
		TargetGroup: op.TargetGroup,
		Status:      op.Status,
	}
	for _, lr := range op.LeafResults {
		status.Total++
		switch lr.Status {
		case "success":
			status.Success++
		case "failed":
			status.Failed++
		default:
			status.Pending++
		}
	}
	return status, nil
}

func generateID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "op-" + hex.EncodeToString(b), nil
}
