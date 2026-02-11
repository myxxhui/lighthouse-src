// Package costmodel provides the core algorithms for calculating dual costs.
package costmodel

import (
	"errors"
	"math"
)

// CalculateCost calculates the dual costs for a Kubernetes resource.
// This is the main entry point for cost calculation.
//
// Input:
//   - rm: Resource metrics (CPU cores, memory bytes)
//   - corePrice: Price per CPU core per hour
//   - memPrice: Price per GB of memory per hour
//
// Output:
//   - CostResult with detailed breakdown
//   - error if validation fails
func CalculateCost(rm ResourceMetric, corePrice, memPrice float64) (CostResult, error) {
	// Validate inputs
	if err := validateInputs(rm, corePrice, memPrice); err != nil {
		return CostResult{}, err
	}

	// Calculate individual costs
	cpuBillable := calcCPUBillable(rm.CPURequest, corePrice)
	cpuUsage := calcCPUUsage(rm.CPUUsageP95, corePrice)
	cpuWaste := calcWaste(cpuBillable, cpuUsage)
	cpuEfficiencyScore := calcCPUEfficiencyScore(rm.CPURequest, rm.CPUUsageP95)

	memBillable := calcMemBillable(rm.MemRequest, memPrice)
	memUsage := calcMemUsage(rm.MemUsageP95, memPrice)
	memWaste := calcWaste(memBillable, memUsage)
	memEfficiencyScore := calcMemEfficiencyScore(rm.MemRequest, rm.MemUsageP95)

	// Calculate overall metrics
	totalBillable := cpuBillable + memBillable
	totalUsage := cpuUsage + memUsage
	totalWaste := totalBillable - totalUsage

	// Calculate overall efficiency score (weighted average)
	overallEfficiencyScore := calcOverallEfficiencyScore(
		cpuEfficiencyScore, memEfficiencyScore,
		cpuBillable, memBillable,
	)

	// Determine grade based on overall efficiency score
	overallGrade := gradeByScore(overallEfficiencyScore)

	// Build result
	result := CostResult{
		CPUBillableCost:    roundToPrecision(cpuBillable, 6),
		CPUUsageCost:       roundToPrecision(cpuUsage, 6),
		CPUWasteCost:       roundToPrecision(cpuWaste, 6),
		CPUEfficiencyScore: roundToPrecision(cpuEfficiencyScore, 2),

		MemBillableCost:    roundToPrecision(memBillable, 6),
		MemUsageCost:       roundToPrecision(memUsage, 6),
		MemWasteCost:       roundToPrecision(memWaste, 6),
		MemEfficiencyScore: roundToPrecision(memEfficiencyScore, 2),

		TotalBillableCost:      roundToPrecision(totalBillable, 6),
		TotalUsageCost:         roundToPrecision(totalUsage, 6),
		TotalWasteCost:         roundToPrecision(totalWaste, 6),
		OverallEfficiencyScore: roundToPrecision(overallEfficiencyScore, 2),
		OverallGrade:           overallGrade,
	}

	return result, nil
}

// validateInputs validates the input parameters.
func validateInputs(rm ResourceMetric, corePrice, memPrice float64) error {
	// Validate resource metrics
	if rm.CPURequest < 0 {
		return errors.New("CPU request cannot be negative")
	}
	if rm.CPUUsageP95 < 0 {
		return errors.New("CPU usage cannot be negative")
	}
	if rm.MemRequest < 0 {
		return errors.New("memory request cannot be negative")
	}
	if rm.MemUsageP95 < 0 {
		return errors.New("memory usage cannot be negative")
	}

	// Validate prices
	if corePrice <= 0 {
		return errors.New("CPU price must be positive")
	}
	if memPrice <= 0 {
		return errors.New("memory price must be positive")
	}

	return nil
}

// calcCPUBillable calculates the billable cost for CPU.
func calcCPUBillable(cpuRequest, corePrice float64) float64 {
	return cpuRequest * corePrice
}

// calcCPUUsage calculates the usage cost for CPU.
func calcCPUUsage(cpuUsageP95, corePrice float64) float64 {
	return cpuUsageP95 * corePrice
}

// calcMemBillable calculates the billable cost for memory.
// Converts bytes to GB for pricing.
func calcMemBillable(memRequest int64, memPrice float64) float64 {
	memGB := float64(memRequest) / (1024 * 1024 * 1024) // Convert bytes to GB
	return memGB * memPrice
}

// calcMemUsage calculates the usage cost for memory.
// Converts bytes to GB for pricing.
func calcMemUsage(memUsageP95 int64, memPrice float64) float64 {
	memGB := float64(memUsageP95) / (1024 * 1024 * 1024) // Convert bytes to GB
	return memGB * memPrice
}

// calcWaste calculates the waste cost.
func calcWaste(billable, usage float64) float64 {
	waste := billable - usage
	if waste < 0 {
		return 0 // Should not happen with valid inputs
	}
	return waste
}

// calcCPUEfficiencyScore calculates the CPU efficiency score (0-100%).
func calcCPUEfficiencyScore(cpuRequest, cpuUsageP95 float64) float64 {
	if cpuRequest == 0 {
		return 100.0 // No request means 100% efficiency
	}
	efficiency := (cpuUsageP95 / cpuRequest) * 100.0

	// Clamp between 0 and 100
	if efficiency > 100.0 {
		return 100.0
	}
	if efficiency < 0 {
		return 0.0
	}
	return efficiency
}

// calcMemEfficiencyScore calculates the memory efficiency score (0-100%).
func calcMemEfficiencyScore(memRequest, memUsageP95 int64) float64 {
	if memRequest == 0 {
		return 100.0 // No request means 100% efficiency
	}
	efficiency := (float64(memUsageP95) / float64(memRequest)) * 100.0

	// Clamp between 0 and 100
	if efficiency > 100.0 {
		return 100.0
	}
	if efficiency < 0 {
		return 0.0
	}
	return efficiency
}

// calcOverallEfficiencyScore calculates the overall efficiency score as a weighted average.
func calcOverallEfficiencyScore(cpuScore, memScore, cpuBillable, memBillable float64) float64 {
	totalBillable := cpuBillable + memBillable
	if totalBillable == 0 {
		return 100.0 // No billable cost means 100% efficiency
	}

	// Weighted average based on billable costs
	weightedScore := (cpuScore*cpuBillable + memScore*memBillable) / totalBillable
	return weightedScore
}

// gradeByScore determines the efficiency grade based on the score.
// Rating standards from the specification document:
// - Zombie (<10%): extremely wasteful, recommend decommission
// - OverProvisioned (10%-40%): over-provisioned, recommend downscaling
// - Healthy (40%-70%): reasonable buffer range
// - Risk (>90%): under-provisioned, OOM risk
//
// Special case: When score is 100% and there are no resource requests,
// it should be considered Healthy.
func gradeByScore(score float64) EfficiencyGrade {
	// Handle special case: 100% efficiency (usually means no request)
	if score == 100.0 {
		return GradeHealthy
	}

	switch {
	case score < 10.0:
		return GradeZombie
	case score < 40.0:
		return GradeOverProvisioned
	case score >= 40.0 && score <= 70.0:
		return GradeHealthy
	case score > 90.0:
		return GradeRisk
	default:
		// For scores between 70% and 90%, we consider them Healthy
		// as they're within reasonable utilization range
		return GradeHealthy
	}
}

// roundToPrecision rounds a float64 value to the specified number of decimal places.
func roundToPrecision(value float64, decimals int) float64 {
	if decimals < 0 {
		return value
	}
	factor := math.Pow(10, float64(decimals))
	return math.Round(value*factor) / factor
}

// FloatEquals compares two float64 values with epsilon tolerance.
// Used for precision validation in tests.
func FloatEquals(a, b, epsilon float64) bool {
	return math.Abs(a-b) <= epsilon
}
