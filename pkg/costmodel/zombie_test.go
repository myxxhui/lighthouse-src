package costmodel

import (
	"math"
	"testing"
)

func TestIsZombie(t *testing.T) {
	tests := []struct {
		name           string
		metrics        ZombieMetrics
		expectedZombie bool
		expectedReason string // optional, can be empty
	}{
		// Standard zombie: all criteria met
		{
			name: "standard zombie",
			metrics: ZombieMetrics{
				CPUAvg:        0.05,
				CPUStdDev:     0.0005,
				MemAvg:        0.05,
				MemStdDev:     0.0005,
				NetworkAvg:    0.5,
				NetworkStdDev: 0.0005,
			},
			expectedZombie: true,
			expectedReason: "CPU, memory, and network usage are consistently below thresholds with minimal variation",
		},
		// Non-zombie: CPU average above threshold
		{
			name: "non-zombie cpu high",
			metrics: ZombieMetrics{
				CPUAvg:        0.2,
				CPUStdDev:     0.0005,
				MemAvg:        0.05,
				MemStdDev:     0.0005,
				NetworkAvg:    0.5,
				NetworkStdDev: 0.0005,
			},
			expectedZombie: false,
			expectedReason: "does not meet zombie criteria: CPU avg (0.200) >= threshold (0.100)",
		},
		// Non-zombie: memory average above threshold
		{
			name: "non-zombie mem high",
			metrics: ZombieMetrics{
				CPUAvg:        0.05,
				CPUStdDev:     0.0005,
				MemAvg:        0.2,
				MemStdDev:     0.0005,
				NetworkAvg:    0.5,
				NetworkStdDev: 0.0005,
			},
			expectedZombie: false,
			expectedReason: "does not meet zombie criteria: memory avg (0.200 GiB) >= threshold (0.100 GiB)",
		},
		// Non-zombie: network average above threshold
		{
			name: "non-zombie network high",
			metrics: ZombieMetrics{
				CPUAvg:        0.05,
				CPUStdDev:     0.0005,
				MemAvg:        0.05,
				MemStdDev:     0.0005,
				NetworkAvg:    2.0,
				NetworkStdDev: 0.0005,
			},
			expectedZombie: false,
			expectedReason: "does not meet zombie criteria: network avg (2.000 KB/s) >= threshold (1.000 KB/s)",
		},
		// Non-zombie: CPU stddev too high (fluctuation)
		{
			name: "non-zombie cpu fluctuating",
			metrics: ZombieMetrics{
				CPUAvg:        0.05,
				CPUStdDev:     0.01,
				MemAvg:        0.05,
				MemStdDev:     0.0005,
				NetworkAvg:    0.5,
				NetworkStdDev: 0.0005,
			},
			expectedZombie: false,
			expectedReason: "does not meet zombie criteria: CPU stddev (0.010) >= dead line (0.001)",
		},
		// Non-zombie: memory stddev too high
		{
			name: "non-zombie mem fluctuating",
			metrics: ZombieMetrics{
				CPUAvg:        0.05,
				CPUStdDev:     0.0005,
				MemAvg:        0.05,
				MemStdDev:     0.01,
				NetworkAvg:    0.5,
				NetworkStdDev: 0.0005,
			},
			expectedZombie: false,
			expectedReason: "does not meet zombie criteria: memory stddev (0.010) >= dead line (0.001)",
		},
		// Non-zombie: network stddev too high
		{
			name: "non-zombie network fluctuating",
			metrics: ZombieMetrics{
				CPUAvg:        0.05,
				CPUStdDev:     0.0005,
				MemAvg:        0.05,
				MemStdDev:     0.0005,
				NetworkAvg:    0.5,
				NetworkStdDev: 0.01,
			},
			expectedZombie: false,
			expectedReason: "does not meet zombie criteria: network stddev (0.010) >= dead line (0.001)",
		},
		// Boundary scenario: CPU exactly at threshold (should be non-zombie because < threshold, not <=)
		{
			name: "boundary cpu equal",
			metrics: ZombieMetrics{
				CPUAvg:        0.1,
				CPUStdDev:     0.0005,
				MemAvg:        0.05,
				MemStdDev:     0.0005,
				NetworkAvg:    0.5,
				NetworkStdDev: 0.0005,
			},
			expectedZombie: false,
			expectedReason: "does not meet zombie criteria: CPU avg (0.100) >= threshold (0.100)",
		},
		// Boundary scenario: memory exactly at threshold
		{
			name: "boundary mem equal",
			metrics: ZombieMetrics{
				CPUAvg:        0.05,
				CPUStdDev:     0.0005,
				MemAvg:        0.1,
				MemStdDev:     0.0005,
				NetworkAvg:    0.5,
				NetworkStdDev: 0.0005,
			},
			expectedZombie: false,
			expectedReason: "does not meet zombie criteria: memory avg (0.100 GiB) >= threshold (0.100 GiB)",
		},
		// Boundary scenario: network exactly at threshold
		{
			name: "boundary network equal",
			metrics: ZombieMetrics{
				CPUAvg:        0.05,
				CPUStdDev:     0.0005,
				MemAvg:        0.05,
				MemStdDev:     0.0005,
				NetworkAvg:    1.0,
				NetworkStdDev: 0.0005,
			},
			expectedZombie: false,
			expectedReason: "does not meet zombie criteria: network avg (1.000 KB/s) >= threshold (1.000 KB/s)",
		},
		// Boundary scenario: stddev exactly at dead line (should be non-zombie because < threshold, not <=)
		{
			name: "boundary stddev equal",
			metrics: ZombieMetrics{
				CPUAvg:        0.05,
				CPUStdDev:     0.001,
				MemAvg:        0.05,
				MemStdDev:     0.0005,
				NetworkAvg:    0.5,
				NetworkStdDev: 0.0005,
			},
			expectedZombie: false,
			expectedReason: "does not meet zombie criteria: CPU stddev (0.001) >= dead line (0.001)",
		},
		// Edge case: all zero (valid zombie)
		{
			name: "all zero",
			metrics: ZombieMetrics{
				CPUAvg:        0.0,
				CPUStdDev:     0.0,
				MemAvg:        0.0,
				MemStdDev:     0.0,
				NetworkAvg:    0.0,
				NetworkStdDev: 0.0,
			},
			expectedZombie: true,
			expectedReason: "CPU, memory, and network usage are consistently below thresholds with minimal variation",
		},
		// Invalid metrics: negative CPU average
		{
			name: "negative cpu",
			metrics: ZombieMetrics{
				CPUAvg:        -0.1,
				CPUStdDev:     0.0005,
				MemAvg:        0.05,
				MemStdDev:     0.0005,
				NetworkAvg:    0.5,
				NetworkStdDev: 0.0005,
			},
			expectedZombie: false,
			expectedReason: "invalid metrics: contains negative values or missing required fields",
		},
		// Invalid metrics: NaN
		{
			name: "nan values",
			metrics: ZombieMetrics{
				CPUAvg:        math.NaN(),
				CPUStdDev:     0.0005,
				MemAvg:        0.05,
				MemStdDev:     0.0005,
				NetworkAvg:    0.5,
				NetworkStdDev: 0.0005,
			},
			expectedZombie: false,
			expectedReason: "invalid metrics: contains negative values or missing required fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isZombie, reason := IsZombie(tt.metrics)
			if isZombie != tt.expectedZombie {
				t.Errorf("IsZombie() zombie status = %v, want %v. Reason: %s", isZombie, tt.expectedZombie, reason)
			}
			// If expectedReason is not empty, we can check that reason contains the expected substring
			if tt.expectedReason != "" && reason != tt.expectedReason {
				// For flexibility, we might just check that reason contains expected keywords.
				// For simplicity, we'll compare exact strings (they should match as we crafted).
				t.Errorf("IsZombie() reason = %q, want %q", reason, tt.expectedReason)
			}
		})
	}
}

func TestCalculateResourceRelease(t *testing.T) {
	tests := []struct {
		name           string
		resource       ResourceMetric
		expectedCPU    float64
		expectedMemory float64
	}{
		{
			name: "typical request",
			resource: ResourceMetric{
				CPURequest: 2.5,
				MemRequest: 4 * 1024 * 1024 * 1024, // 4 GiB in bytes
			},
			expectedCPU:    2.5,
			expectedMemory: 4.0,
		},
		{
			name: "zero request",
			resource: ResourceMetric{
				CPURequest: 0.0,
				MemRequest: 0,
			},
			expectedCPU:    0.0,
			expectedMemory: 0.0,
		},
		{
			name: "fractional cpu",
			resource: ResourceMetric{
				CPURequest: 0.25,
				MemRequest: 536870912, // 0.5 GiB
			},
			expectedCPU:    0.25,
			expectedMemory: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpu, mem := CalculateResourceRelease(tt.resource)
			if !floatEqual(cpu, tt.expectedCPU) {
				t.Errorf("CalculateResourceRelease() CPU = %v, want %v", cpu, tt.expectedCPU)
			}
			if !floatEqual(mem, tt.expectedMemory) {
				t.Errorf("CalculateResourceRelease() memory = %v GiB, want %v GiB", mem, tt.expectedMemory)
			}
		})
	}
}

func TestGenerateOptimizationSuggestion(t *testing.T) {
	// Create a zombie metrics
	zombieMetrics := ZombieMetrics{
		CPUAvg:        0.05,
		CPUStdDev:     0.0005,
		MemAvg:        0.05,
		MemStdDev:     0.0005,
		NetworkAvg:    0.5,
		NetworkStdDev: 0.0005,
	}
	resource := ResourceMetric{
		CPURequest: 2.0,
		MemRequest: 2 * 1024 * 1024 * 1024, // 2 GiB
	}

	suggestion := GenerateOptimizationSuggestion(zombieMetrics, resource)
	if suggestion == "" {
		t.Error("GenerateOptimizationSuggestion() returned empty string")
	}
	// Should contain key phrases
	expectedPhrases := []string{"Zombie detected", "scale down", "terminate", "resource release", "cores", "GiB"}
	for _, phrase := range expectedPhrases {
		if !contains(suggestion, phrase) {
			t.Errorf("GenerateOptimizationSuggestion() missing phrase %q in suggestion: %s", phrase, suggestion)
		}
	}

	// Test non-zombie case
	nonZombieMetrics := ZombieMetrics{
		CPUAvg:        0.2,
		CPUStdDev:     0.0005,
		MemAvg:        0.05,
		MemStdDev:     0.0005,
		NetworkAvg:    0.5,
		NetworkStdDev: 0.0005,
	}
	suggestion2 := GenerateOptimizationSuggestion(nonZombieMetrics, resource)
	if !contains(suggestion2, "Not a zombie") {
		t.Errorf("GenerateOptimizationSuggestion() for non-zombie should contain 'Not a zombie', got: %s", suggestion2)
	}
}

func TestVerifyZombieDetection(t *testing.T) {
	metrics := ZombieMetrics{
		CPUAvg:        0.05,
		CPUStdDev:     0.0005,
		MemAvg:        0.05,
		MemStdDev:     0.0005,
		NetworkAvg:    0.5,
		NetworkStdDev: 0.0005,
	}
	passed, msg := VerifyZombieDetection(metrics, true, "")
	if !passed {
		t.Errorf("VerifyZombieDetection() failed: %s", msg)
	}

	// Mismatch case
	passed2, msg2 := VerifyZombieDetection(metrics, false, "")
	if passed2 {
		t.Errorf("VerifyZombieDetection() should have failed but passed")
	} else {
		t.Logf("Expected failure: %s", msg2)
	}
}

// Helper functions
func floatEqual(a, b float64) bool {
	const epsilon = 1e-9
	return math.Abs(a-b) < epsilon
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}
