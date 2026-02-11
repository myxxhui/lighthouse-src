package costmodel

import (
	"math"
	"testing"
	"time"
)

// TestCalculateCost tests the main CalculateCost function with various scenarios.
func TestCalculateCost(t *testing.T) {
	// Constants for testing
	const corePrice = 0.025 // $0.025 per core per hour
	const memPrice = 0.01   // $0.01 per GB per hour

	// Helper to convert GB to bytes
	gbToBytes := func(gb float64) int64 {
		return int64(gb * 1024 * 1024 * 1024)
	}

	testCases := []struct {
		name           string
		input          ResourceMetric
		corePrice      float64
		memPrice       float64
		expected       CostResult
		expectError    bool
		expectedErrMsg string
	}{
		// Standard scenario: Request=2.0, Usage=1.0 → Score=50%, Grade=Healthy
		{
			name: "标准场景: CPU和内存使用率50%",
			input: ResourceMetric{
				CPURequest:  2.0,
				CPUUsageP95: 1.0,
				MemRequest:  gbToBytes(2.0), // 2GB
				MemUsageP95: gbToBytes(1.0), // 1GB
				Timestamp:   time.Now(),
			},
			expected: CostResult{
				// CPU costs
				CPUBillableCost:    2.0 * corePrice, // 0.05
				CPUUsageCost:       1.0 * corePrice, // 0.025
				CPUWasteCost:       1.0 * corePrice, // 0.025
				CPUEfficiencyScore: 50.0,            // (1.0/2.0)*100

				// Memory costs
				MemBillableCost:    2.0 * memPrice, // 0.02
				MemUsageCost:       1.0 * memPrice, // 0.01
				MemWasteCost:       1.0 * memPrice, // 0.01
				MemEfficiencyScore: 50.0,           // (1.0/2.0)*100

				// Total costs
				TotalBillableCost:      0.07,  // 0.05 + 0.02
				TotalUsageCost:         0.035, // 0.025 + 0.01
				TotalWasteCost:         0.035, // 0.025 + 0.01
				OverallEfficiencyScore: 50.0,  // Weighted average
				OverallGrade:           GradeHealthy,
			},
			expectError: false,
		},

		// Zombie scenario: Request=4.0, Usage=0.05 → Score=1.25%, Grade=Zombie
		{
			name: "僵尸场景: 极低使用率",
			input: ResourceMetric{
				CPURequest:  4.0,
				CPUUsageP95: 0.05,
				MemRequest:  gbToBytes(4.0),  // 4GB
				MemUsageP95: gbToBytes(0.05), // 0.05GB
				Timestamp:   time.Now(),
			},
			expected: CostResult{
				// CPU costs
				CPUBillableCost:    4.0 * corePrice,  // 0.1
				CPUUsageCost:       0.05 * corePrice, // 0.00125
				CPUWasteCost:       3.95 * corePrice, // 0.09875
				CPUEfficiencyScore: 1.25,             // (0.05/4.0)*100

				// Memory costs
				MemBillableCost:    4.0 * memPrice,  // 0.04
				MemUsageCost:       0.05 * memPrice, // 0.0005
				MemWasteCost:       3.95 * memPrice, // 0.0395
				MemEfficiencyScore: 1.25,            // (0.05/4.0)*100

				// Total costs
				TotalBillableCost:      0.14,    // 0.1 + 0.04
				TotalUsageCost:         0.00175, // 0.00125 + 0.0005
				TotalWasteCost:         0.13825, // 0.09875 + 0.0395
				OverallEfficiencyScore: 1.25,    // Weighted average
				OverallGrade:           GradeZombie,
			},
			expectError: false,
		},

		// Risk scenario: Request=1.0, Usage=0.95 → Score=95%, Grade=Risk
		{
			name: "风险场景: 高使用率接近极限",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: 0.95,
				MemRequest:  gbToBytes(1.0),  // 1GB
				MemUsageP95: gbToBytes(0.95), // 0.95GB
				Timestamp:   time.Now(),
			},
			expected: CostResult{
				// CPU costs
				CPUBillableCost:    1.0 * corePrice,  // 0.025
				CPUUsageCost:       0.95 * corePrice, // 0.02375
				CPUWasteCost:       0.05 * corePrice, // 0.00125
				CPUEfficiencyScore: 95.0,             // (0.95/1.0)*100

				// Memory costs
				MemBillableCost:    1.0 * memPrice,  // 0.01
				MemUsageCost:       0.95 * memPrice, // 0.0095
				MemWasteCost:       0.05 * memPrice, // 0.0005
				MemEfficiencyScore: 95.0,            // (0.95/1.0)*100

				// Total costs
				TotalBillableCost:      0.035,   // 0.025 + 0.01
				TotalUsageCost:         0.03325, // 0.02375 + 0.0095
				TotalWasteCost:         0.00175, // 0.00125 + 0.0005
				OverallEfficiencyScore: 95.0,    // Weighted average
				OverallGrade:           GradeRisk,
			},
			expectError: false,
		},

		// Zero request scenario: Request=0 → EfficiencyScore=100%, Grade=Healthy
		{
			name: "零请求场景: 没有资源请求",
			input: ResourceMetric{
				CPURequest:  0.0,
				CPUUsageP95: 0.0,
				MemRequest:  gbToBytes(0.0), // 0GB
				MemUsageP95: gbToBytes(0.0), // 0GB
				Timestamp:   time.Now(),
			},
			expected: CostResult{
				// CPU costs
				CPUBillableCost:    0.0,
				CPUUsageCost:       0.0,
				CPUWasteCost:       0.0,
				CPUEfficiencyScore: 100.0, // No request means 100% efficiency

				// Memory costs
				MemBillableCost:    0.0,
				MemUsageCost:       0.0,
				MemWasteCost:       0.0,
				MemEfficiencyScore: 100.0, // No request means 100% efficiency

				// Total costs
				TotalBillableCost:      0.0,
				TotalUsageCost:         0.0,
				TotalWasteCost:         0.0,
				OverallEfficiencyScore: 100.0, // Weighted average
				OverallGrade:           GradeHealthy,
			},
			expectError: false,
		},

		// Precision test scenario: Floating point precision validation
		{
			name: "精度测试: 验证误差<1%",
			input: ResourceMetric{
				CPURequest:  1.234,
				CPUUsageP95: 0.987,
				MemRequest:  gbToBytes(2.345), // 2.345GB
				MemUsageP95: gbToBytes(1.876), // 1.876GB
				Timestamp:   time.Now(),
			},
			expected: CostResult{
				// CPU costs (calculated)
				CPUBillableCost:    1.234 * corePrice,     // 0.03085
				CPUUsageCost:       0.987 * corePrice,     // 0.024675
				CPUWasteCost:       0.247 * corePrice,     // 0.006175
				CPUEfficiencyScore: (0.987 / 1.234) * 100, // ~79.98%

				// Memory costs (calculated)
				MemBillableCost:    2.345 * memPrice,      // 0.02345
				MemUsageCost:       1.876 * memPrice,      // 0.01876
				MemWasteCost:       0.469 * memPrice,      // 0.00469
				MemEfficiencyScore: (1.876 / 2.345) * 100, // ~79.99%

				// Total costs
				TotalBillableCost: 0.03085 + 0.02345,  // 0.0543
				TotalUsageCost:    0.024675 + 0.01876, // 0.043435
				TotalWasteCost:    0.006175 + 0.00469, // 0.010865

				// Overall efficiency score (weighted average)
				// Weighted by billable costs: (79.98*0.03085 + 79.99*0.02345) / (0.03085+0.02345)
				OverallEfficiencyScore: 79.985, // Approximate
				OverallGrade:           GradeHealthy,
			},
			expectError: false,
		},

		// Error scenario: Negative CPU request
		{
			name: "错误场景: 负的CPU请求",
			input: ResourceMetric{
				CPURequest:  -1.0,
				CPUUsageP95: 1.0,
				MemRequest:  gbToBytes(1.0),
				MemUsageP95: gbToBytes(1.0),
				Timestamp:   time.Now(),
			},
			expectError:    true,
			expectedErrMsg: "CPU request cannot be negative",
		},

		// Error scenario: Zero price
		{
			name: "错误场景: 零价格",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: 0.5,
				MemRequest:  gbToBytes(1.0),
				MemUsageP95: gbToBytes(0.5),
				Timestamp:   time.Now(),
			},
			corePrice:      0.0, // Invalid price
			memPrice:       0.01,
			expectError:    true,
			expectedErrMsg: "CPU price must be positive",
		},

		// Error scenario: Negative memory usage
		{
			name: "错误场景: 负的内存使用量",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: 0.5,
				MemRequest:  gbToBytes(1.0),
				MemUsageP95: -1, // Invalid negative memory usage
				Timestamp:   time.Now(),
			},
			expectError:    true,
			expectedErrMsg: "memory usage cannot be negative",
		},

		// Error scenario: Negative memory request
		{
			name: "错误场景: 负的内存请求",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: 0.5,
				MemRequest:  -1, // Invalid negative memory request
				MemUsageP95: gbToBytes(0.5),
				Timestamp:   time.Now(),
			},
			expectError:    true,
			expectedErrMsg: "memory request cannot be negative",
		},

		// Error scenario: Negative CPU usage
		{
			name: "错误场景: 负的CPU使用量",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: -0.5, // Invalid negative CPU usage
				MemRequest:  gbToBytes(1.0),
				MemUsageP95: gbToBytes(0.5),
				Timestamp:   time.Now(),
			},
			expectError:    true,
			expectedErrMsg: "CPU usage cannot be negative",
		},

		// Error scenario: Zero memory price
		{
			name: "错误场景: 零内存价格",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: 0.5,
				MemRequest:  gbToBytes(1.0),
				MemUsageP95: gbToBytes(0.5),
				Timestamp:   time.Now(),
			},
			corePrice:      0.025,
			memPrice:       0.0, // Invalid price
			expectError:    true,
			expectedErrMsg: "memory price must be positive",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Use default prices if not specified in test case
			// But don't override if test expects an error (price validation is being tested)
			corePrice := tc.corePrice
			if corePrice == 0 && !tc.expectError {
				corePrice = 0.025
			}
			memPrice := tc.memPrice
			if memPrice == 0 && !tc.expectError {
				memPrice = 0.01
			}

			result, err := CalculateCost(tc.input, corePrice, memPrice)

			// Check error expectations
			if tc.expectError {
				if err == nil {
					t.Errorf("期望错误但没有得到错误")
				} else if tc.expectedErrMsg != "" && err.Error() != tc.expectedErrMsg {
					t.Errorf("期望错误消息 %q, 得到 %q", tc.expectedErrMsg, err.Error())
				}
				return
			}

			// Check no error expected
			if err != nil {
				t.Errorf("不期望错误但得到: %v", err)
				return
			}

			// For precision test, we use epsilon comparison
			if tc.name == "精度测试: 验证误差<1%" {
				validatePrecision(t, result, tc.expected)
				return
			}

			// Validate results with tolerance
			const tolerance = 0.0001 // 0.01% tolerance for floating point

			// Validate CPU costs
			if !FloatEquals(result.CPUBillableCost, tc.expected.CPUBillableCost, tolerance) {
				t.Errorf("CPUBillableCost 不匹配: 期望 %.6f, 得到 %.6f",
					tc.expected.CPUBillableCost, result.CPUBillableCost)
			}
			if !FloatEquals(result.CPUUsageCost, tc.expected.CPUUsageCost, tolerance) {
				t.Errorf("CPUUsageCost 不匹配: 期望 %.6f, 得到 %.6f",
					tc.expected.CPUUsageCost, result.CPUUsageCost)
			}
			if !FloatEquals(result.CPUWasteCost, tc.expected.CPUWasteCost, tolerance) {
				t.Errorf("CPUWasteCost 不匹配: 期望 %.6f, 得到 %.6f",
					tc.expected.CPUWasteCost, result.CPUWasteCost)
			}
			if !FloatEquals(result.CPUEfficiencyScore, tc.expected.CPUEfficiencyScore, tolerance) {
				t.Errorf("CPUEfficiencyScore 不匹配: 期望 %.6f, 得到 %.6f",
					tc.expected.CPUEfficiencyScore, result.CPUEfficiencyScore)
			}

			// Validate Memory costs
			if !FloatEquals(result.MemBillableCost, tc.expected.MemBillableCost, tolerance) {
				t.Errorf("MemBillableCost 不匹配: 期望 %.6f, 得到 %.6f",
					tc.expected.MemBillableCost, result.MemBillableCost)
			}
			if !FloatEquals(result.MemUsageCost, tc.expected.MemUsageCost, tolerance) {
				t.Errorf("MemUsageCost 不匹配: 期望 %.6f, 得到 %.6f",
					tc.expected.MemUsageCost, result.MemUsageCost)
			}
			if !FloatEquals(result.MemWasteCost, tc.expected.MemWasteCost, tolerance) {
				t.Errorf("MemWasteCost 不匹配: 期望 %.6f, 得到 %.6f",
					tc.expected.MemWasteCost, result.MemWasteCost)
			}
			if !FloatEquals(result.MemEfficiencyScore, tc.expected.MemEfficiencyScore, tolerance) {
				t.Errorf("MemEfficiencyScore 不匹配: 期望 %.6f, 得到 %.6f",
					tc.expected.MemEfficiencyScore, result.MemEfficiencyScore)
			}

			// Validate Total costs
			if !FloatEquals(result.TotalBillableCost, tc.expected.TotalBillableCost, tolerance) {
				t.Errorf("TotalBillableCost 不匹配: 期望 %.6f, 得到 %.6f",
					tc.expected.TotalBillableCost, result.TotalBillableCost)
			}
			if !FloatEquals(result.TotalUsageCost, tc.expected.TotalUsageCost, tolerance) {
				t.Errorf("TotalUsageCost 不匹配: 期望 %.6f, 得到 %.6f",
					tc.expected.TotalUsageCost, result.TotalUsageCost)
			}
			if !FloatEquals(result.TotalWasteCost, tc.expected.TotalWasteCost, tolerance) {
				t.Errorf("TotalWasteCost 不匹配: 期望 %.6f, 得到 %.6f",
					tc.expected.TotalWasteCost, result.TotalWasteCost)
			}

			// Validate efficiency score with slightly higher tolerance for weighted average
			const scoreTolerance = 0.01 // 0.01% for weighted average
			if !FloatEquals(result.OverallEfficiencyScore, tc.expected.OverallEfficiencyScore, scoreTolerance) {
				t.Errorf("OverallEfficiencyScore 不匹配: 期望 %.6f, 得到 %.6f",
					tc.expected.OverallEfficiencyScore, result.OverallEfficiencyScore)
			}

			// Validate grade
			if result.OverallGrade != tc.expected.OverallGrade {
				t.Errorf("OverallGrade 不匹配: 期望 %v, 得到 %v",
					tc.expected.OverallGrade, result.OverallGrade)
			}
		})
	}
}

// validatePrecision validates that calculation errors are less than 1%.
func validatePrecision(t *testing.T, actual, expected CostResult) {
	const maxErrorPercent = 1.0 // 1% maximum error

	// Helper to calculate percentage error
	calcErrorPercent := func(actual, expected float64) float64 {
		if expected == 0 {
			if actual == 0 {
				return 0
			}
			return 100.0 // Both should be 0
		}
		return math.Abs((actual - expected) / expected * 100.0)
	}

	// Check each field for precision
	fields := []struct {
		name     string
		actual   float64
		expected float64
	}{
		{"CPUBillableCost", actual.CPUBillableCost, expected.CPUBillableCost},
		{"CPUUsageCost", actual.CPUUsageCost, expected.CPUUsageCost},
		{"CPUWasteCost", actual.CPUWasteCost, expected.CPUWasteCost},
		{"CPUEfficiencyScore", actual.CPUEfficiencyScore, expected.CPUEfficiencyScore},
		{"MemBillableCost", actual.MemBillableCost, expected.MemBillableCost},
		{"MemUsageCost", actual.MemUsageCost, expected.MemUsageCost},
		{"MemWasteCost", actual.MemWasteCost, expected.MemWasteCost},
		{"MemEfficiencyScore", actual.MemEfficiencyScore, expected.MemEfficiencyScore},
		{"TotalBillableCost", actual.TotalBillableCost, expected.TotalBillableCost},
		{"TotalUsageCost", actual.TotalUsageCost, expected.TotalUsageCost},
		{"TotalWasteCost", actual.TotalWasteCost, expected.TotalWasteCost},
		{"OverallEfficiencyScore", actual.OverallEfficiencyScore, expected.OverallEfficiencyScore},
	}

	for _, field := range fields {
		errorPercent := calcErrorPercent(field.actual, field.expected)
		if errorPercent > maxErrorPercent {
			t.Errorf("精度验证失败 %s: 误差 %.4f%% 超过 %.1f%% 限制 (实际: %.6f, 期望: %.6f)",
				field.name, errorPercent, maxErrorPercent, field.actual, field.expected)
		}
	}
}

// TestGradeByScore tests the gradeByScore function independently.
func TestGradeByScore(t *testing.T) {
	testCases := []struct {
		name     string
		score    float64
		expected EfficiencyGrade
	}{
		{"极低分: Zombie", 5.0, GradeZombie},
		{"边界: 刚好低于10%", 9.9, GradeZombie},
		{"边界: 10%", 10.0, GradeOverProvisioned},
		{"中等偏低: OverProvisioned", 25.0, GradeOverProvisioned},
		{"边界: 刚好低于40%", 39.9, GradeOverProvisioned},
		{"边界: 40%", 40.0, GradeHealthy},
		{"健康范围: Healthy", 55.0, GradeHealthy},
		{"边界: 刚好低于70%", 69.9, GradeHealthy},
		{"边界: 70%", 70.0, GradeHealthy}, // 70-90% is also considered Healthy
		{"高使用率但未超限", 85.0, GradeHealthy},
		{"边界: 刚好高于90%", 90.1, GradeRisk},
		{"高风险: Risk", 95.0, GradeRisk},
		{"满分: 100%", 100.0, GradeHealthy}, // 100% is special case
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := gradeByScore(tc.score)
			if result != tc.expected {
				t.Errorf("gradeByScore(%.1f) = %v, 期望 %v", tc.score, result, tc.expected)
			}
		})
	}
}

// TestEfficiencyScoreFunctions tests the individual efficiency score calculation functions.
func TestEfficiencyScoreFunctions(t *testing.T) {
	t.Run("CPU效率分计算", func(t *testing.T) {
		// Test normal case
		score := calcCPUEfficiencyScore(2.0, 1.0)
		expected := 50.0
		if !FloatEquals(score, expected, 0.001) {
			t.Errorf("calcCPUEfficiencyScore(2.0, 1.0) = %.6f, 期望 %.6f", score, expected)
		}

		// Test zero request
		score = calcCPUEfficiencyScore(0.0, 0.5)
		expected = 100.0
		if !FloatEquals(score, expected, 0.001) {
			t.Errorf("calcCPUEfficiencyScore(0.0, 0.5) = %.6f, 期望 %.6f", score, expected)
		}

		// Test zero request and zero usage
		score = calcCPUEfficiencyScore(0.0, 0.0)
		expected = 100.0
		if !FloatEquals(score, expected, 0.001) {
			t.Errorf("calcCPUEfficiencyScore(0.0, 0.0) = %.6f, 期望 %.6f", score, expected)
		}

		// Test usage exceeds request
		score = calcCPUEfficiencyScore(1.0, 1.5)
		expected = 100.0 // Clamped to 100%
		if !FloatEquals(score, expected, 0.001) {
			t.Errorf("calcCPUEfficiencyScore(1.0, 1.5) = %.6f, 期望 %.6f", score, expected)
		}

		// Test usage equals request (100% efficiency)
		score = calcCPUEfficiencyScore(2.0, 2.0)
		expected = 100.0
		if !FloatEquals(score, expected, 0.001) {
			t.Errorf("calcCPUEfficiencyScore(2.0, 2.0) = %.6f, 期望 %.6f", score, expected)
		}

		// Test negative usage (should be clamped to 0)
		score = calcCPUEfficiencyScore(2.0, -1.0)
		expected = 0.0
		if !FloatEquals(score, expected, 0.001) {
			t.Errorf("calcCPUEfficiencyScore(2.0, -1.0) = %.6f, 期望 %.6f", score, expected)
		}
	})

	t.Run("内存效率分计算", func(t *testing.T) {
		// Test normal case
		score := calcMemEfficiencyScore(2048, 1024) // 2GB request, 1GB usage
		expected := 50.0
		if !FloatEquals(score, expected, 0.001) {
			t.Errorf("calcMemEfficiencyScore(2048, 1024) = %.6f, 期望 %.6f", score, expected)
		}

		// Test zero request
		score = calcMemEfficiencyScore(0, 1024)
		expected = 100.0
		if !FloatEquals(score, expected, 0.001) {
			t.Errorf("calcMemEfficiencyScore(0, 1024) = %.6f, 期望 %.6f", score, expected)
		}

		// Test zero request and zero usage
		score = calcMemEfficiencyScore(0, 0)
		expected = 100.0
		if !FloatEquals(score, expected, 0.001) {
			t.Errorf("calcMemEfficiencyScore(0, 0) = %.6f, 期望 %.6f", score, expected)
		}

		// Test usage equals request (100% efficiency)
		score = calcMemEfficiencyScore(2048, 2048)
		expected = 100.0
		if !FloatEquals(score, expected, 0.001) {
			t.Errorf("calcMemEfficiencyScore(2048, 2048) = %.6f, 期望 %.6f", score, expected)
		}

		// Test usage exceeds request (should be clamped to 100%)
		score = calcMemEfficiencyScore(1024, 2048)
		expected = 100.0
		if !FloatEquals(score, expected, 0.001) {
			t.Errorf("calcMemEfficiencyScore(1024, 2048) = %.6f, 期望 %.6f", score, expected)
		}

		// Test negative usage (should be clamped to 0%)
		score = calcMemEfficiencyScore(2048, -1024)
		expected = 0.0
		if !FloatEquals(score, expected, 0.001) {
			t.Errorf("calcMemEfficiencyScore(2048, -1024) = %.6f, 期望 %.6f", score, expected)
		}
	})

	t.Run("整体效率分计算", func(t *testing.T) {
		// Test weighted average
		score := calcOverallEfficiencyScore(50.0, 70.0, 100.0, 50.0)
		// Weighted average: (50*100 + 70*50) / (100+50) = (5000 + 3500) / 150 = 8500/150 = 56.666...
		expected := 56.666666666666664
		if !FloatEquals(score, expected, 0.001) {
			t.Errorf("calcOverallEfficiencyScore(50, 70, 100, 50) = %.6f, 期望 %.6f", score, expected)
		}

		// Test zero billable cost
		score = calcOverallEfficiencyScore(50.0, 70.0, 0.0, 0.0)
		expected = 100.0
		if !FloatEquals(score, expected, 0.001) {
			t.Errorf("calcOverallEfficiencyScore(50, 70, 0, 0) = %.6f, 期望 %.6f", score, expected)
		}
	})
}

// TestCostCalculationFunctions tests the individual cost calculation functions.
func TestCostCalculationFunctions(t *testing.T) {
	const corePrice = 0.025
	const memPrice = 0.01

	t.Run("CPU成本计算", func(t *testing.T) {
		billable := calcCPUBillable(2.0, corePrice)
		expected := 0.05
		if !FloatEquals(billable, expected, 0.0001) {
			t.Errorf("calcCPUBillable(2.0, %.3f) = %.6f, 期望 %.6f", corePrice, billable, expected)
		}

		usage := calcCPUUsage(1.0, corePrice)
		expected = 0.025
		if !FloatEquals(usage, expected, 0.0001) {
			t.Errorf("calcCPUUsage(1.0, %.3f) = %.6f, 期望 %.6f", corePrice, usage, expected)
		}
	})

	t.Run("内存成本计算", func(t *testing.T) {
		// 2GB memory
		billable := calcMemBillable(gbToBytes(2.0), memPrice)
		expected := 0.02
		if !FloatEquals(billable, expected, 0.0001) {
			t.Errorf("calcMemBillable(2GB, %.3f) = %.6f, 期望 %.6f", memPrice, billable, expected)
		}

		// 1GB usage
		usage := calcMemUsage(gbToBytes(1.0), memPrice)
		expected = 0.01
		if !FloatEquals(usage, expected, 0.0001) {
			t.Errorf("calcMemUsage(1GB, %.3f) = %.6f, 期望 %.6f", memPrice, usage, expected)
		}
	})

	t.Run("浪费成本计算", func(t *testing.T) {
		waste := calcWaste(0.1, 0.07)
		expected := 0.03
		if !FloatEquals(waste, expected, 0.0001) {
			t.Errorf("calcWaste(0.1, 0.07) = %.6f, 期望 %.6f", waste, expected)
		}

		// Waste should not be negative
		waste = calcWaste(0.05, 0.1)
		expected = 0.0
		if !FloatEquals(waste, expected, 0.0001) {
			t.Errorf("calcWaste(0.05, 0.1) = %.6f, 期望 %.6f", waste, expected)
		}
	})

	t.Run("精度舍入函数", func(t *testing.T) {
		// Test rounding to different decimal places
		if result := roundToPrecision(3.1415926535, 0); result != 3.0 {
			t.Errorf("roundToPrecision(3.1415926535, 0) = %.6f, 期望 3.0", result)
		}
		if result := roundToPrecision(3.1415926535, 2); result != 3.14 {
			t.Errorf("roundToPrecision(3.1415926535, 2) = %.6f, 期望 3.14", result)
		}
		if result := roundToPrecision(3.1415926535, 4); result != 3.1416 {
			t.Errorf("roundToPrecision(3.1415926535, 4) = %.6f, 期望 3.1416", result)
		}

		// Test negative decimals (should return original value)
		if result := roundToPrecision(3.14159, -1); result != 3.14159 {
			t.Errorf("roundToPrecision(3.14159, -1) = %.6f, 期望 3.14159", result)
		}

		// Test zero value
		if result := roundToPrecision(0.0, 2); result != 0.0 {
			t.Errorf("roundToPrecision(0.0, 2) = %.6f, 期望 0.0", result)
		}

		// Test negative value
		if result := roundToPrecision(-3.14159, 2); result != -3.14 {
			t.Errorf("roundToPrecision(-3.14159, 2) = %.6f, 期望 -3.14", result)
		}
	})
}

// TestFloatEquals tests the FloatEquals utility function.
func TestFloatEquals(t *testing.T) {
	testCases := []struct {
		a, b, epsilon float64
		expected      bool
	}{
		{1.0, 1.0, 0.001, true},
		{1.0, 1.001, 0.01, true},
		{1.0, 1.002, 0.001, false},
		{0.0, 0.0, 0.0001, true},
		{-1.0, -1.0, 0.001, true},
		{100.0, 100.05, 0.1, true},
	}

	for _, tc := range testCases {
		result := FloatEquals(tc.a, tc.b, tc.epsilon)
		if result != tc.expected {
			t.Errorf("FloatEquals(%.6f, %.6f, %.6f) = %v, 期望 %v",
				tc.a, tc.b, tc.epsilon, result, tc.expected)
		}
	}
}

// Helper function for tests
func gbToBytes(gb float64) int64 {
	return int64(gb * 1024 * 1024 * 1024)
}
