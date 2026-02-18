// Package postgres provides repository implementations for PostgreSQL storage.
// namespace_repo.go: interface for cost_daily_namespace access (L2 控制平面).
package postgres

import (
	"context"
	"time"
)

// NamespaceDailyRepo defines access to cost_daily_namespace. Phase1: placeholder.
// Full implementation may embed Repository or use Repository.ListDailyNamespaceCosts.
type NamespaceDailyRepo interface {
	// ListByDateRange lists daily namespace costs in date range. Phase2 implement.
	ListByDateRange(ctx context.Context, start, end time.Time) ([]DailyNamespaceCost, error)
}
