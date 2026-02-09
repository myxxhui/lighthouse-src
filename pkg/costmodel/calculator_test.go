package costmodel

import (
	"math"
	"testing"
	"time"
)

func TestCalculateCost(t *testing.T) {
	testCases := []struct {
		name           string
		input          ResourceMetric
		corePrice      float64
		memPrice       float64
		resourceType   string
		expected       CostResult
		expectError    bool
		expectedErrMsg string
	}{
		{
			name: "标准场景: Request=2.0, Usage=1.0",
			input: ResourceMetric{
				CPURequest:  2.0,
				CPUUsageP95: 1.0,
				MemRequest:  2.0,
				MemUsageP95: 1.0,
				Timestamp:   time.Now(),
			},
			corePrice:    0.025,
			memPrice:     0.01,
			resourceType: "cpu",
			expected: CostResult{
				BillableCost:    0.07,  // (2*0.025 + 2*0.01) = 0.05 + 0.02 = 0.07
				UsageCost:       0.035, // (1*0.025 + 1*0.01) = 0.025 + 0.01 = 0.035
				WasteCost:       0.035,
				EfficiencyScore: 50.0, // (1+1)/(2+2) * 100 = 50%
				Grade:           Healthy,
				ResourceType:    "cpu",
			},
		},
		{
			name: "僵尸场景: Request=4.0, Usage=0.05",
			input: ResourceMetric{
				CPURequest:  4.0,
				CPUUsageP95: 0.05,
				MemRequest:  4.0,
				MemUsageP95: 0.05,
				Timestamp:   time.Now(),
			},
			corePrice:    0.025,
			memPrice:     0.01,
			resourceType: "memory",
			expected: CostResult{
				BillableCost:    0.14,   // (4*0.025 + 4*0.01) = 0.1 + 0.04 = 0.14
				UsageCost:       0.0015, // (0.05*0.025 + 0.05*0.01) = 0.00125 + 0.0005 = 0.00175 ≈ 0.0018
				WasteCost:       0.1385,
				EfficiencyScore: 1.25, // (0.05+0.05)/(4+4) * 100 = 1.25%
				Grade:           Zombie,
				ResourceType:    "memory",
			},
		},
		{
			name: "风险场景: Request=1.0, Usage=0.95",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: 0.95,
				MemRequest:  1.0,
				MemUsageP95: 0.95,
				Timestamp:   time.Now(),
			},
			corePrice:    0.025,
			memPrice:     0.01,
			resourceType: "cpu",
			expected: CostResult{
				BillableCost:    0.035,   // (1*0.025 + 1*0.01) = 0.025 + 0.01 = 0.035
				UsageCost:       0.03325, // (0.95*0.025 + 0.95*0.01) = 0.02375 + 0.0095 = 0.03325
				WasteCost:       0.00175,
				EfficiencyScore: 95.0, // (0.95+0.95)/(1+1) * 100 = 95%
				Grade:           Risk,
				ResourceType:    "cpu",
			},
		},
		{
			name: "除零处理: Request=0",
			input: ResourceMetric{
				CPURequest:  0.0,
				CPUUsageP95: 0.0,
				MemRequest:  0.0,
				MemUsageP95: 0.0,
				Timestamp:   time.Now(),
			},
			corePrice:    0.025,
			memPrice:     0.01,
			resourceType: "cpu",
			expected: CostResult{
				BillableCost:    0.0,
				UsageCost:       0.0,
				WasteCost:       0.0,
				EfficiencyScore: 100.0,
				Grade:           Healthy,
				ResourceType:    "cpu",
			},
		},
		{
			name: "精度测试: 小数计算",
			input: ResourceMetric{
				CPURequest:  1.234,
				CPUUsageP95: 0.987,
				MemRequest:  2.345,
				MemUsageP95: 1.876,
				Timestamp:   time.Now(),
			},
			corePrice:    0.025,
			memPrice:     0.01,
			resourceType: "memory",
			expected: CostResult{
				BillableCost:    0.055, // (1.234*0.025 + 2.345*0.01) = 0.03085 + 0.02345 = 0.0543 ≈ 0.05
				UsageCost:       0.043, // (0.987*0.025 + 1.876*0.01) = 0.024675 + 0.01876 = 0.043435 ≈ 0.04
				WasteCost:       0.012,
				EfficiencyScore: 79.99, // (0.987+1.876)/(1.234+2.345) * 100 ≈ 79.99%
				Grade:           Healthy,
				ResourceType:    "memory",
			},
		},
		{
			name: "无效输入: 负数",
			input: ResourceMetric{
				CPURequest:  -1.0,
				CPUUsageP95: 1.0,
				MemRequest:  2.0,
				MemUsageP95: 1.0,
				Timestamp:   time.Now(),
			},
			corePrice:      0.025,
			memPrice:       0.01,
			resourceType:   "cpu",
			expectError:    true,
			expectedErrMsg: "resource metrics cannot be negative",
		},
		{
			name: "无效输入: 价格为零",
			input: ResourceMetric{
				CPURequest:  2.0,
				CPUUsageP95: 1.0,
				MemRequest:  2.0,
				MemUsageP95: 1.0,
				Timestamp:   time.Now(),
			},
			corePrice:      0.0,
			memPrice:       0.01,
			resourceType:   "cpu",
			expectError:    true,
			expectedErrMsg: "price must be positive",
		},
		{
			name: "边界条件: 效率分正好40%",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: 0.4,
				MemRequest:  1.0,
				MemUsageP95: 0.4,
				Timestamp:   time.Now(),
			},
			corePrice:    0.025,
			memPrice:     0.01,
			resourceType: "cpu",
			expected: CostResult{
				BillableCost:    0.035,
				UsageCost:       0.014,
				WasteCost:       0.021,
				EfficiencyScore: 40.0,
				Grade:           Healthy,
				ResourceType:    "cpu",
			},
		},
		{
			name: "边界条件: 效率分正好70%",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: 0.7,
				MemRequest:  1.0,
				MemUsageP95: 0.7,
				Timestamp:   time.Now(),
			},
			corePrice:    0.025,
			memPrice:     0.01,
			resourceType: "cpu",
			expected: CostResult{
				BillableCost:    0.035,
				UsageCost:       0.0245,
				WasteCost:       0.0105,
				EfficiencyScore: 70.0,
				Grade:           Healthy,
				ResourceType:    "cpu",
			},
		},
		{
			name: "边界条件: 效率分正好90%",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: 0.9,
				MemRequest:  1.0,
				MemUsageP95: 0.9,
				Timestamp:   time.Now(),
			},
			corePrice:    0.025,
			memPrice:     0.01,
			resourceType: "cpu",
			expected: CostResult{
				BillableCost:    0.035,
				UsageCost:       0.0315,
				WasteCost:       0.0035,
				EfficiencyScore: 90.0,
				Grade:           Healthy,
				ResourceType:    "cpu",
			},
		},
		{
			name: "边界条件: 效率分正好95%",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: 0.95,
				MemRequest:  1.0,
				MemUsageP95: 0.95,
				Timestamp:   time.Now(),
			},
			corePrice:    0.025,
			memPrice:     0.01,
			resourceType: "cpu",
			expected: CostResult{
				BillableCost:    0.035,
				UsageCost:       0.03325,
				WasteCost:       0.00175,
				EfficiencyScore: 95.0,
				Grade:           Risk,
				ResourceType:    "cpu",
			},
		},
		{
			name: "边界中间值: 9.999%",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: 0.09999,
				MemRequest:  1.0,
				MemUsageP95: 0.09999,
				Timestamp:   time.Now(),
			},
			corePrice:    0.025,
			memPrice:     0.01,
			resourceType: "cpu",
			expected: CostResult{
				BillableCost:    0.035,
				UsageCost:       0.0035,
				WasteCost:       0.0315,
				EfficiencyScore: 10.0,
				Grade:           OverProvisioned,
				ResourceType:    "cpu",
			},
		},
		{
			name: "边界中间值: 39.999%",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: 0.39999,
				MemRequest:  1.0,
				MemUsageP95: 0.39999,
				Timestamp:   time.Now(),
			},
			corePrice:    0.025,
			memPrice:     0.01,
			resourceType: "cpu",
			expected: CostResult{
				BillableCost:    0.035,
				UsageCost:       0.014,
				WasteCost:       0.021,
				EfficiencyScore: 40.0,
				Grade:           Healthy,
				ResourceType:    "cpu",
			},
		},
		{
			name: "边界中间值: 89.999%",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: 0.89999,
				MemRequest:  1.0,
				MemUsageP95: 0.89999,
				Timestamp:   time.Now(),
			},
			corePrice:    0.025,
			memPrice:     0.01,
			resourceType: "cpu",
			expected: CostResult{
				BillableCost:    0.035,
				UsageCost:       0.031499,
				WasteCost:       0.003501,
				EfficiencyScore: 89.999,
				Grade:           Healthy,
				ResourceType:    "cpu",
			},
		},
		{
			name: "边界中间值: 90.001%",
			input: ResourceMetric{
				CPURequest:  1.0,
				CPUUsageP95: 0.90001,
				MemRequest:  1.0,
				MemUsageP95: 0.90001,
				Timestamp:   time.Now(),
			},
			corePrice:    0.025,
			memPrice:     0.01,
			resourceType: "cpu",
			expected: CostResult{
				BillableCost:    0.035,
				UsageCost:       0.0315,
				WasteCost:       0.0035,
				EfficiencyScore: 90.0,
				Grade:           Healthy,
				ResourceType:    "cpu",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := CalculateCost(tc.input, tc.corePrice, tc.memPrice, tc.resourceType)

			if tc.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if err.Error() != tc.expectedErrMsg {
					t.Errorf("expected error message '%s', got '%s'", tc.expectedErrMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// 验证精度误差 <1%
			if !withinTolerance(result.BillableCost, tc.expected.BillableCost, 0.01) {
				t.Errorf("BillableCost expected %v, got %v", tc.expected.BillableCost, result.BillableCost)
			}

			if !withinTolerance(result.UsageCost, tc.expected.UsageCost, 0.01) {
				t.Errorf("UsageCost expected %v, got %v", tc.expected.UsageCost, result.UsageCost)
			}

			if !withinTolerance(result.WasteCost, tc.expected.WasteCost, 0.01) {
				t.Errorf("WasteCost expected %v, got %v", tc.expected.WasteCost, result.WasteCost)
			}

			if !withinTolerance(result.EfficiencyScore, tc.expected.EfficiencyScore, 1.0) {
				t.Errorf("EfficiencyScore expected %v, got %v (input: CPU=%v, Mem=%v)", tc.expected.EfficiencyScore, result.EfficiencyScore, tc.input.CPUUsageP95, tc.input.MemUsageP95)
			}

			if result.Grade != tc.expected.Grade {
				t.Errorf("Grade expected %v, got %v (score: %v)", tc.expected.Grade, result.Grade, result.EfficiencyScore)
			}

			if result.ResourceType != tc.expected.ResourceType {
				t.Errorf("ResourceType expected %v, got %v", tc.expected.ResourceType, result.ResourceType)
			}
		})
	}
}

// withinTolerance 检查实际值是否在预期值的容差范围内
func withinTolerance(actual, expected, tolerance float64) bool {
	diff := math.Abs(actual - expected)
	return diff <= tolerance
}
