package sync

import "go.temporal.io/sdk/worker"

// RegisterWorker registers the sync workflow and activities on a worker.
func RegisterWorker(w worker.Worker, a *Activities) {
	w.RegisterWorkflow(SyncUserDataWorkflow)
	w.RegisterActivity(a)
}
