// Package postgres provides repository implementations for PostgreSQL storage.
// hourly_repo.go: interface for cost_hourly_workload access (L2 信号→控制平面).
package postgres

import (
	"context"
	"time"
)

// HourlyWorkloadRepo defines access to cost_hourly_workload. Phase1: placeholder.
// Full implementation may embed Repository or use Repository.ListHourlyWorkloadStats.
type HourlyWorkloadRepo interface {
	// ListByTimeRange lists hourly workload stats in time range. Phase2 implement.
	ListByTimeRange(ctx context.Context, start, end time.Time) ([]HourlyWorkloadStat, error)
}
