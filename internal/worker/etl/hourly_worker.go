// Package etl provides ETL workers for hourly and daily aggregation.
// hourly_worker.go: placeholder for L2 hourly workload ETL (cost_hourly_workload).
package etl

// HourlyWorker runs hourly aggregation from signal plane to control plane.
// Phase1: placeholder only; no business logic.
type HourlyWorker struct{}

// Run is a placeholder. Implementation in Phase2.
func (w *HourlyWorker) Run() error { return nil }
