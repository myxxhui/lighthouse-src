// Package costmodel provides the core types and algorithms for calculating
// dual costs (billable vs usage) in Kubernetes resource analysis.
package costmodel

import (
	"fmt"
	"math"
)

const (
	// Zombie detection thresholds
	cpuThreshold     = 0.1   // CPU average usage < 0.1 core
	memThreshold     = 0.1   // Memory average usage < 0.1 GiB
	networkThreshold = 1.0   // Network average IO < 1 KB/s
	stdDevThreshold  = 0.001 // Standard deviation threshold for "dead line"
)

// IsZombie determines whether a resource is a zombie based on 7-day usage statistics.
// It returns a boolean indicating zombie status and a string describing the reason.
// The function is pure and has no side effects.
func IsZombie(metrics ZombieMetrics) (bool, string) {
	// Validate input
	if !isValidZombieMetrics(metrics) {
		return false, "invalid metrics: contains negative values or missing required fields"
	}

	// Check CPU criteria
	cpuLow := metrics.CPUAvg < cpuThreshold
	cpuDead := metrics.CPUStdDev < stdDevThreshold
	cpuOk := cpuLow && cpuDead

	// Check memory criteria (convert GiB to bytes if needed, but metrics are in GiB)
	memLow := metrics.MemAvg < memThreshold
	memDead := metrics.MemStdDev < stdDevThreshold
	memOk := memLow && memDead

	// Check network criteria (network threshold is 1 KB/s)
	networkLow := metrics.NetworkAvg < networkThreshold
	// Network standard deviation may be ignored, but we can still check for stability
	networkDead := metrics.NetworkStdDev < stdDevThreshold
	networkOk := networkLow && networkDead

	// All three dimensions must meet criteria
	if cpuOk && memOk && networkOk {
		return true, "CPU, memory, and network usage are consistently below thresholds with minimal variation"
	}

	// Build reason for non-zombie
	reasons := []string{}
	if !cpuLow {
		reasons = append(reasons, fmt.Sprintf("CPU avg (%.3f) >= threshold (%.3f)", metrics.CPUAvg, cpuThreshold))
	}
	if !cpuDead {
		reasons = append(reasons, fmt.Sprintf("CPU stddev (%.3f) >= dead line (%.3f)", metrics.CPUStdDev, stdDevThreshold))
	}
	if !memLow {
		reasons = append(reasons, fmt.Sprintf("memory avg (%.3f GiB) >= threshold (%.3f GiB)", metrics.MemAvg, memThreshold))
	}
	if !memDead {
		reasons = append(reasons, fmt.Sprintf("memory stddev (%.3f) >= dead line (%.3f)", metrics.MemStdDev, stdDevThreshold))
	}
	if !networkLow {
		reasons = append(reasons, fmt.Sprintf("network avg (%.3f KB/s) >= threshold (%.3f KB/s)", metrics.NetworkAvg, networkThreshold))
	}
	if !networkDead {
		reasons = append(reasons, fmt.Sprintf("network stddev (%.3f) >= dead line (%.3f)", metrics.NetworkStdDev, stdDevThreshold))
	}

	reason := "does not meet zombie criteria: " + joinReasons(reasons)
	return false, reason
}

// CalculateResourceRelease computes the amount of CPU (cores) and memory (GiB) that can be released
// from a zombie resource. Input is a ResourceMetric containing requested resources.
// Returns releasable CPU cores and memory GiB.
func CalculateResourceRelease(resource ResourceMetric) (float64, float64) {
	// For a zombie, we assume the entire requested amount can be released
	// because usage is negligible.
	// However, we might want to keep a safety buffer (e.g., 10% of request).
	// For simplicity, we release 100% of requested resources.
	// Convert memory from bytes to GiB (1 GiB = 1024 * 1024 * 1024 bytes)
	const bytesPerGiB = 1024 * 1024 * 1024
	releasableCPU := resource.CPURequest
	releasableMem := float64(resource.MemRequest) / bytesPerGiB
	return releasableCPU, releasableMem
}

// GenerateOptimizationSuggestion generates a human-readable suggestion for optimizing a zombie resource.
func GenerateOptimizationSuggestion(metrics ZombieMetrics, resource ResourceMetric) string {
	isZombie, reason := IsZombie(metrics)
	if !isZombie {
		return fmt.Sprintf("Not a zombie: %s", reason)
	}
	cpu, mem := CalculateResourceRelease(resource)
	return fmt.Sprintf("Zombie detected. Suggested action: scale down or terminate. "+
		"Potential resource release: %.2f cores, %.2f GiB memory. "+
		"Cost savings estimated based on waste billable cost.", cpu, mem)
}

// isValidZombieMetrics validates that the metrics contain reasonable values.
// It checks for negative numbers and ensures required fields are present.
func isValidZombieMetrics(metrics ZombieMetrics) bool {
	// Check for negative values (except std dev can be zero)
	if metrics.CPUAvg < 0 || metrics.CPUStdDev < 0 ||
		metrics.MemAvg < 0 || metrics.MemStdDev < 0 ||
		metrics.NetworkAvg < 0 || metrics.NetworkStdDev < 0 {
		return false
	}
	// Check for NaN or Inf
	if math.IsNaN(metrics.CPUAvg) || math.IsInf(metrics.CPUAvg, 0) ||
		math.IsNaN(metrics.CPUStdDev) || math.IsInf(metrics.CPUStdDev, 0) ||
		math.IsNaN(metrics.MemAvg) || math.IsInf(metrics.MemAvg, 0) ||
		math.IsNaN(metrics.MemStdDev) || math.IsInf(metrics.MemStdDev, 0) ||
		math.IsNaN(metrics.NetworkAvg) || math.IsInf(metrics.NetworkAvg, 0) ||
		math.IsNaN(metrics.NetworkStdDev) || math.IsInf(metrics.NetworkStdDev, 0) {
		return false
	}
	return true
}

// joinReasons concatenates a slice of reasons with semicolons.
func joinReasons(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	result := reasons[0]
	for i := 1; i < len(reasons); i++ {
		result += "; " + reasons[i]
	}
	return result
}

// VerifyZombieDetection is a tool function to verify the correctness of zombie detection.
// It compares the actual detection result with the expected result and returns a verification report.
// This is useful for validation and testing.
func VerifyZombieDetection(metrics ZombieMetrics, expectedIsZombie bool, expectedReason string) (bool, string) {
	actualIsZombie, actualReason := IsZombie(metrics)
	if actualIsZombie != expectedIsZombie {
		return false, fmt.Sprintf("detection mismatch: expected zombie=%v, got zombie=%v. Reason: %s", expectedIsZombie, actualIsZombie, actualReason)
	}
	// Optionally compare reason strings, but reasons may vary in wording.
	// For simplicity, we only check if both reasons are non-empty.
	if expectedReason != "" && actualReason != expectedReason {
		// Log mismatch but not considered failure
		return true, fmt.Sprintf("detection correct but reason mismatch: expected '%s', got '%s'", expectedReason, actualReason)
	}
	return true, "verification passed"
}
