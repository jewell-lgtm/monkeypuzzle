package sync

import (
	"context"
	"fmt"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
)

// SyncTrigger starts a user's sync and reports its status. The web process
// depends on this interface; the test substitutes a synchronous runner.
type SyncTrigger interface {
	StartSync(ctx context.Context, userID int64) (runID string, err error)
	SyncStatus(ctx context.Context, userID int64) (store.SyncStatus, error)
}

// TemporalTrigger starts SyncUserDataWorkflow via a Temporal client.
type TemporalTrigger struct {
	client client.Client
	store  store.Store
}

// NewTemporalTrigger wires a trigger to a Temporal client and the store.
func NewTemporalTrigger(c client.Client, s store.Store) *TemporalTrigger {
	return &TemporalTrigger{client: c, store: s}
}

// StartSync starts (or restarts) the per-user sync. A deterministic workflow ID
// plus TERMINATE_EXISTING coalesces repeated triggers into one fresh run.
func (t *TemporalTrigger) StartSync(ctx context.Context, userID int64) (string, error) {
	opts := client.StartWorkflowOptions{
		ID:                       fmt.Sprintf("sync-user-%d", userID),
		TaskQueue:                TaskQueue,
		WorkflowIDConflictPolicy: enumspb.WORKFLOW_ID_CONFLICT_POLICY_TERMINATE_EXISTING,
	}
	we, err := t.client.ExecuteWorkflow(ctx, opts, SyncUserDataWorkflow, SyncInput{UserID: userID})
	if err != nil {
		return "", fmt.Errorf("sync: start workflow: %w", err)
	}
	runID := we.GetRunID()
	if err := t.store.SetSyncStatus(ctx, userID, store.SyncStatus{Status: store.SyncRunning, LastRunID: runID}); err != nil {
		return runID, fmt.Errorf("sync: set running status: %w", err)
	}
	return runID, nil
}

// SyncStatus returns the user's current sync status from the store.
func (t *TemporalTrigger) SyncStatus(ctx context.Context, userID int64) (store.SyncStatus, error) {
	return t.store.GetSyncStatus(ctx, userID)
}

var _ SyncTrigger = (*TemporalTrigger)(nil)
