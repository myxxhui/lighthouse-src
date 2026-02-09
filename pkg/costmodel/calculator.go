package costmodel

import (
	"errors"
	"math"
)

// CalculateCost 计算双重成本
// rm: 资源指标
// corePrice: CPU核心小时成本
// memPrice: 内存GB小时成本
// resourceType: 资源类型 ("cpu" 或 "memory")
func CalculateCost(rm ResourceMetric, corePrice, memPrice float64, resourceType string) (CostResult, error) {
	// 验证输入
	if err := validateResourceMetric(rm, corePrice, memPrice); err != nil {
		return CostResult{}, err
	}

	// 计算账单成本
	billable := calcBillable(rm, corePrice, memPrice)

	// 计算使用价值
	usage := calcUsage(rm, corePrice, memPrice)

	// 计算浪费金额
	waste := calcWaste(billable, usage)

	// 计算效率分
	efficiencyScore := calcEfficiencyScore(rm)

	// 评级
	grade := gradeByScore(efficiencyScore)

	return CostResult{
		BillableCost:    billable,
		UsageCost:       usage,
		WasteCost:       waste,
		EfficiencyScore: efficiencyScore,
		Grade:           grade,
		ResourceType:    resourceType,
	}, nil
}

func validateResourceMetric(rm ResourceMetric, corePrice, memPrice float64) error {
	if rm.CPURequest < 0 || rm.CPUUsageP95 < 0 || rm.MemRequest < 0 || rm.MemUsageP95 < 0 {
		return errors.New("resource metrics cannot be negative")
	}
	if corePrice <= 0 || memPrice <= 0 {
		return errors.New("price must be positive")
	}
	return nil
}

func calcBillable(rm ResourceMetric, corePrice, memPrice float64) float64 {
	cpuCost := rm.CPURequest * corePrice
	memCost := rm.MemRequest * memPrice
	return roundToTwoDecimal(cpuCost + memCost)
}

func calcUsage(rm ResourceMetric, corePrice, memPrice float64) float64 {
	cpuCost := rm.CPUUsageP95 * corePrice
	memCost := rm.MemUsageP95 * memPrice
	return roundToTwoDecimal(cpuCost + memCost)
}

func calcWaste(billable, usage float64) float64 {
	return roundToTwoDecimal(billable - usage)
}

func calcEfficiencyScore(rm ResourceMetric) float64 {
	// 计算总请求量
	totalRequest := rm.CPURequest + rm.MemRequest
	if totalRequest == 0 {
		return 100.0
	}

	// 计算总使用量
	totalUsage := rm.CPUUsageP95 + rm.MemUsageP95

	// 计算效率分
	efficiency := (totalUsage / totalRequest) * 100.0
	return roundToTwoDecimal(efficiency)
}

func gradeByScore(score float64) EfficiencyGrade {
	// 特殊处理：当效率分为100%且没有请求资源时，视为Healthy
	if score == 100.0 {
		return Healthy
	}

	switch {
	case score < 10.0:
		return Zombie
	case score < 40.0:
		return OverProvisioned
	case score < 70.0:
		return Healthy
	case score > 90.0:
		return Risk
	default:
		return Healthy
	}
}

func roundToTwoDecimal(value float64) float64 {
	return math.Round(value*100) / 100
}
