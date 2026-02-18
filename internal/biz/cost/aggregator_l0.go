// Package cost defines the business domain types and interfaces for cost calculation.
// aggregator_l0.go: L0 (global) aggregation; 全域聚合占位，具体算法见 pkg/costmodel.AggregateGlobal.
package cost

import (
	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

// L0Aggregator represents the global-level (L0) aggregator for cluster-wide cost view.
// Phase1: placeholder; use costmodel.AggregateGlobal for L0 aggregation in Phase2.
type L0Aggregator struct{}

// Level returns LevelCluster for L0 global aggregation.
func (a *L0Aggregator) Level() costmodel.AggregationLevel {
	return costmodel.LevelCluster
}
