// Package costmodel provides the core algorithms for calculating dual costs.
package costmodel

import (
	"errors"
	"math"
	"sort"
	"time"
)

// AggregateGlobal aggregates daily namespace costs into global view (L0).
// This function MUST use DailyNamespaceCost data from daily_namespace_costs table.
//
// Input: []DailyNamespaceCost (data from daily_namespace_costs table)
// Output: GlobalAggregatedResult with total billable cost, total waste, and global efficiency
func AggregateGlobal(costs []DailyNamespaceCost) (GlobalAggregatedResult, error) {
	if len(costs) == 0 {
		return GlobalAggregatedResult{
			Timestamp: time.Now(),
		}, nil
	}

	var totalBillable, totalUsage, totalWaste float64

	for _, cost := range costs {
		totalBillable += cost.BillableCost
		totalUsage += cost.UsageCost
		totalWaste += cost.WasteCost
	}

	// Calculate global efficiency: (total usage / total billable) * 100%
	var globalEfficiency float64
	if totalBillable > 0 {
		globalEfficiency = (totalUsage / totalBillable) * 100.0
	}

	// Round to 2 decimal places for financial precision
	totalBillable = roundFinancial(totalBillable)
	totalWaste = roundFinancial(totalWaste)
	globalEfficiency = roundPercentage(globalEfficiency)

	return GlobalAggregatedResult{
		TotalBillableCost: totalBillable,
		TotalWaste:        totalWaste,
		GlobalEfficiency:  globalEfficiency,
		Timestamp:         time.Now(),
	}, nil
}

// CalculateDomainBreakdown calculates the cost breakdown by namespace/domain for pie chart (L0).
// This function MUST use DailyNamespaceCost data from daily_namespace_costs table.
//
// Input: []DailyNamespaceCost (data from daily_namespace_costs table)
// Output: []DomainBreakdownItem with cost percentages for each namespace
func CalculateDomainBreakdown(costs []DailyNamespaceCost) ([]DomainBreakdownItem, error) {
	if len(costs) == 0 {
		return []DomainBreakdownItem{}, nil
	}

	// First, aggregate by namespace (sum costs across multiple days)
	namespaceCosts := make(map[string]*DailyNamespaceCost)

	for _, cost := range costs {
		if existing, exists := namespaceCosts[cost.Namespace]; exists {
			existing.BillableCost += cost.BillableCost
			existing.UsageCost += cost.UsageCost
			existing.WasteCost += cost.WasteCost
			existing.PodCount += cost.PodCount
			existing.NodeCount += cost.NodeCount
			existing.WorkloadCount += cost.WorkloadCount
		} else {
			namespaceCosts[cost.Namespace] = &DailyNamespaceCost{
				Namespace:     cost.Namespace,
				BillableCost:  cost.BillableCost,
				UsageCost:     cost.UsageCost,
				WasteCost:     cost.WasteCost,
				PodCount:      cost.PodCount,
				NodeCount:     cost.NodeCount,
				WorkloadCount: cost.WorkloadCount,
			}
		}
	}

	// Calculate total billable cost for percentage calculation
	var totalBillable float64
	for _, cost := range namespaceCosts {
		totalBillable += cost.BillableCost
	}

	// Create breakdown items
	var breakdown []DomainBreakdownItem

	for namespace, cost := range namespaceCosts {
		var costPercentage float64
		if totalBillable > 0 {
			costPercentage = (cost.BillableCost / totalBillable) * 100.0
		}

		breakdown = append(breakdown, DomainBreakdownItem{
			DomainName:     namespace,
			CostPercentage: roundPercentage(costPercentage),
			BillableCost:   roundFinancial(cost.BillableCost),
			UsageCost:      roundFinancial(cost.UsageCost),
			WasteCost:      roundFinancial(cost.WasteCost),
			PodCount:       cost.PodCount,
		})
	}

	// Sort by cost percentage descending
	sort.Slice(breakdown, func(i, j int) bool {
		return breakdown[i].CostPercentage > breakdown[j].CostPercentage
	})

	return breakdown, nil
}

// AggregateByNamespace aggregates hourly workload stats by namespace (L1).
//
// Input: []HourlyWorkloadStat (data from hourly_workload_stats table)
// Output: map[string]AggregatedResult keyed by namespace name
func AggregateByNamespace(stats []HourlyWorkloadStat) (map[string]AggregatedResult, error) {
	if len(stats) == 0 {
		return make(map[string]AggregatedResult), nil
	}

	namespaceAggregates := make(map[string]*aggregateData)

	for _, stat := range stats {
		ns := stat.Namespace
		if _, exists := namespaceAggregates[ns]; !exists {
			namespaceAggregates[ns] = &aggregateData{}
		}

		agg := namespaceAggregates[ns]
		agg.totalBillable += stat.TotalBillableCost
		agg.totalUsage += stat.TotalUsageCost
		agg.totalWaste += stat.TotalWasteCost
		agg.resourceCount++
	}

	// Convert to AggregatedResult map
	result := make(map[string]AggregatedResult)

	for namespace, agg := range namespaceAggregates {
		efficiencyScore := calculateEfficiencyScore(agg.totalBillable, agg.totalUsage)

		result[namespace] = AggregatedResult{
			Identifier:        namespace,
			TotalBillableCost: roundFinancial(agg.totalBillable),
			TotalUsageCost:    roundFinancial(agg.totalUsage),
			TotalWasteCost:    roundFinancial(agg.totalWaste),
			EfficiencyScore:   roundPercentage(efficiencyScore),
			ResourceCount:     agg.resourceCount,
			Timestamp:         time.Now(),
		}
	}

	return result, nil
}

// AggregateByNode aggregates cost results by node (L2).
//
// Input: []CostResult (real-time Prometheus data or from hourly table)
// Output: map[string]AggregatedResult keyed by node name
func AggregateByNode(costs []CostResult, nodeNames []string) (map[string]AggregatedResult, error) {
	if len(costs) == 0 {
		return make(map[string]AggregatedResult), nil
	}

	if len(costs) != len(nodeNames) {
		return nil, errors.New("costs and nodeNames must have same length")
	}

	nodeAggregates := make(map[string]*aggregateData)

	for i, cost := range costs {
		nodeName := nodeNames[i]
		if _, exists := nodeAggregates[nodeName]; !exists {
			nodeAggregates[nodeName] = &aggregateData{}
		}

		agg := nodeAggregates[nodeName]
		agg.totalBillable += cost.TotalBillableCost
		agg.totalUsage += cost.TotalUsageCost
		agg.totalWaste += cost.TotalWasteCost
		agg.resourceCount++
	}

	// Convert to AggregatedResult map
	result := make(map[string]AggregatedResult)

	for nodeName, agg := range nodeAggregates {
		efficiencyScore := calculateEfficiencyScore(agg.totalBillable, agg.totalUsage)

		result[nodeName] = AggregatedResult{
			Identifier:        nodeName,
			TotalBillableCost: roundFinancial(agg.totalBillable),
			TotalUsageCost:    roundFinancial(agg.totalUsage),
			TotalWasteCost:    roundFinancial(agg.totalWaste),
			EfficiencyScore:   roundPercentage(efficiencyScore),
			ResourceCount:     agg.resourceCount,
			Timestamp:         time.Now(),
		}
	}

	return result, nil
}

// AggregateByWorkload aggregates hourly workload stats by workload (L3).
//
// Input: []HourlyWorkloadStat (data from hourly_workload_stats table)
// Output: map[string]AggregatedResult keyed by workload identifier (namespace/workloadName)
func AggregateByWorkload(stats []HourlyWorkloadStat) (map[string]AggregatedResult, error) {
	if len(stats) == 0 {
		return make(map[string]AggregatedResult), nil
	}

	workloadAggregates := make(map[string]*aggregateData)

	for _, stat := range stats {
		workloadID := stat.Namespace + "/" + stat.WorkloadName
		if _, exists := workloadAggregates[workloadID]; !exists {
			workloadAggregates[workloadID] = &aggregateData{}
		}

		agg := workloadAggregates[workloadID]
		agg.totalBillable += stat.TotalBillableCost
		agg.totalUsage += stat.TotalUsageCost
		agg.totalWaste += stat.TotalWasteCost
		agg.resourceCount++
	}

	// Convert to AggregatedResult map
	result := make(map[string]AggregatedResult)

	for workloadID, agg := range workloadAggregates {
		efficiencyScore := calculateEfficiencyScore(agg.totalBillable, agg.totalUsage)

		result[workloadID] = AggregatedResult{
			Identifier:        workloadID,
			TotalBillableCost: roundFinancial(agg.totalBillable),
			TotalUsageCost:    roundFinancial(agg.totalUsage),
			TotalWasteCost:    roundFinancial(agg.totalWaste),
			EfficiencyScore:   roundPercentage(efficiencyScore),
			ResourceCount:     agg.resourceCount,
			Timestamp:         time.Now(),
		}
	}

	return result, nil
}

// AggregateByPod aggregates cost results by pod (L4).
//
// Input: []CostResult (real-time Prometheus data)
// Output: map[string]AggregatedResult keyed by pod identifier (namespace/podName)
func AggregateByPod(costs []CostResult, podIDs []string) (map[string]AggregatedResult, error) {
	if len(costs) == 0 {
		return make(map[string]AggregatedResult), nil
	}

	if len(costs) != len(podIDs) {
		return nil, errors.New("costs and podIDs must have same length")
	}

	podAggregates := make(map[string]*aggregateData)

	for i, cost := range costs {
		podID := podIDs[i]
		if _, exists := podAggregates[podID]; !exists {
			podAggregates[podID] = &aggregateData{}
		}

		agg := podAggregates[podID]
		agg.totalBillable += cost.TotalBillableCost
		agg.totalUsage += cost.TotalUsageCost
		agg.totalWaste += cost.TotalWasteCost
		agg.resourceCount++
	}

	// Convert to AggregatedResult map
	result := make(map[string]AggregatedResult)

	for podID, agg := range podAggregates {
		efficiencyScore := calculateEfficiencyScore(agg.totalBillable, agg.totalUsage)

		result[podID] = AggregatedResult{
			Identifier:        podID,
			TotalBillableCost: roundFinancial(agg.totalBillable),
			TotalUsageCost:    roundFinancial(agg.totalUsage),
			TotalWasteCost:    roundFinancial(agg.totalWaste),
			EfficiencyScore:   roundPercentage(efficiencyScore),
			ResourceCount:     agg.resourceCount,
			Timestamp:         time.Now(),
		}
	}

	return result, nil
}

// Helper functions

// aggregateData is an internal structure for accumulating aggregation data
type aggregateData struct {
	totalBillable float64
	totalUsage    float64
	totalWaste    float64
	resourceCount int
}

// calculateEfficiencyScore calculates efficiency score from billable and usage costs
// Efficiency = (usage / billable) * 100% (0-100 scale)
func calculateEfficiencyScore(billable, usage float64) float64 {
	if billable <= 0 || usage < 0 {
		return 0.0
	}

	// Cap usage at billable (can't be more than 100% efficient)
	if usage > billable {
		usage = billable
	}

	return (usage / billable) * 100.0
}

// roundFinancial rounds a float64 to financial precision (2 decimal places)
func roundFinancial(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0.0
	}

	return math.Round(value*100) / 100
}

// roundPercentage rounds a percentage value (2 decimal places)
func roundPercentage(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0.0
	}

	return math.Round(value*100) / 100
}

// validateCostInput validates cost inputs for negative values
func validateCostInput(costs []DailyNamespaceCost) error {
	for _, cost := range costs {
		if cost.BillableCost < 0 {
			return errors.New("billable cost cannot be negative")
		}
		if cost.UsageCost < 0 {
			return errors.New("usage cost cannot be negative")
		}
		if cost.WasteCost < 0 {
			return errors.New("waste cost cannot be negative")
		}
	}
	return nil
}

// validateWorkloadStatInput validates workload stat inputs for negative values
func validateWorkloadStatInput(stats []HourlyWorkloadStat) error {
	for _, stat := range stats {
		if stat.TotalBillableCost < 0 {
			return errors.New("total billable cost cannot be negative")
		}
		if stat.TotalUsageCost < 0 {
			return errors.New("total usage cost cannot be negative")
		}
		if stat.TotalWasteCost < 0 {
			return errors.New("total waste cost cannot be negative")
		}
	}
	return nil
}
