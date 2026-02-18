// Package etl provides ETL workers for hourly and daily aggregation.
// daily_worker.go: placeholder for L2 daily namespace ETL (cost_daily_namespace).
package etl

// DailyWorker runs daily aggregation from signal plane to control plane.
// Phase1: placeholder only; no business logic.
type DailyWorker struct{}

// Run is a placeholder. Implementation in Phase2.
func (w *DailyWorker) Run() error { return nil }
