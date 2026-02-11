//go:build ignore
// +build ignore

// This file is for type verification only, not part of the main build
package main

import (
	"fmt"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/biz/cost"
	"github.com/myxxhui/lighthouse-src/internal/biz/roi"
	"github.com/myxxhui/lighthouse-src/internal/biz/slo"
	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

func main() {
	fmt.Println("=== Lighthouse 类型系统验证 ===")

	// 1. 验证costmodel包类型
	fmt.Println("\n1. 验证costmodel包类型:")
	testCostModelTypes()

	// 2. 验证cost包类型
	fmt.Println("\n2. 验证cost包类型:")
	testCostTypes()

	// 3. 验证slo包类型
	fmt.Println("\n3. 验证slo包类型:")
	testSLOTypes()

	// 4. 验证roi包类型
	fmt.Println("\n4. 验证roi包类型:")
	testROITypes()

	fmt.Println("\n✅ 所有类型定义验证通过!")
}

func testCostModelTypes() {
	// 测试ResourceMetric
	rm := costmodel.ResourceMetric{
		CPURequest:  2.5,
		CPUUsageP95: 1.2,
		MemRequest:  4 * 1024 * 1024 * 1024, // 4GB
		MemUsageP95: 2 * 1024 * 1024 * 1024, // 2GB
		Timestamp:   time.Now(),
	}
	fmt.Printf("  ResourceMetric创建成功: CPU请求=%.2f, 内存请求=%.2fGB\n",
		rm.CPURequest, float64(rm.MemRequest)/(1024*1024*1024))

	// 测试DualCostResult
	dcr := costmodel.DualCostResult{
		CPUBillableCost:        100.0,
		CPUUsageCost:           60.0,
		CPUWasteCost:           40.0,
		CPUEfficiencyScore:     60.0,
		MemBillableCost:        80.0,
		MemUsageCost:           50.0,
		MemWasteCost:           30.0,
		MemEfficiencyScore:     62.5,
		TotalBillableCost:      180.0,
		TotalUsageCost:         110.0,
		TotalWasteCost:         70.0,
		OverallEfficiencyScore: 61.25,
		CPUGrade:               costmodel.Healthy,
		MemGrade:               costmodel.Healthy,
		OverallGrade:           costmodel.Healthy,
		CalculatedAt:           time.Now(),
		PrecisionError:         0.005,
	}
	fmt.Printf("  DualCostResult创建成功: 总浪费成本=%.2f, 总效率分=%.2f%%\n",
		dcr.TotalWasteCost, dcr.OverallEfficiencyScore)

	// 测试PrecisionConfig
	pc := costmodel.DefaultPrecisionConfig()
	fmt.Printf("  PrecisionConfig创建成功: 成本精度=%d位小数, 最大误差=%.2f%%\n",
		pc.CostDecimalPlaces, pc.MaxErrorPercentage*100)

	// 测试AggregationLevel
	fmt.Printf("  聚合层级: %s, %s, %s, %s\n",
		costmodel.LevelNamespace, costmodel.LevelNode,
		costmodel.LevelWorkload, costmodel.LevelPod)

	// 测试EfficiencyGrade阈值
	fmt.Printf("  效率分等级阈值: Zombie(<%.2f%%), Healthy(%.2f%%-%.2f%%)\n",
		costmodel.GradeThresholds[costmodel.Zombie].Max,
		costmodel.GradeThresholds[costmodel.Healthy].Min,
		costmodel.GradeThresholds[costmodel.Healthy].Max)
}

func testCostTypes() {
	// 测试聚合器工厂
	factory := &cost.AggregatorFactory{}

	// 测试Namespace聚合器
	nsAggregator := factory.CreateAggregator(
		costmodel.LevelNamespace,
		"production",
		map[string]string{},
	)
	fmt.Printf("  Namespace聚合器创建成功: 层级=%s\n", nsAggregator.Level())

	// 测试聚合上下文
	ctx := cost.NewAggregationContext(nsAggregator, []costmodel.DualCostResult{})
	fmt.Printf("  聚合上下文创建成功: 时间戳=%v\n", ctx.Timestamp.Format("2006-01-02 15:04:05"))
}

func testSLOTypes() {
	// 测试AvailabilityScore
	as := slo.AvailabilityScore{
		StartTime:              time.Now().Add(-24 * time.Hour),
		EndTime:                time.Now(),
		TotalRequests:          1000000,
		SuccessfulRequests:     999900,
		FailedRequests:         100,
		AvailabilityPercentage: 99.99,
		TargetSLO:              99.9,
		ComplianceStatus:       slo.SLOStatusHealthy,
		ErrorBudgetConsumed:    0.1,
		ErrorBudgetRemaining:   99.9,
		BurnRate:               0.01,
	}
	fmt.Printf("  AvailabilityScore创建成功: 可用性=%.4f%%, 目标SLO=%.1f%%\n",
		as.AvailabilityPercentage, as.TargetSLO)

	// 测试LatencyP95
	latency := slo.LatencyP95{
		StartTime:           time.Now().Add(-1 * time.Hour),
		EndTime:             time.Now(),
		SampleCount:         10000,
		P50:                 50.0,
		P75:                 75.0,
		P90:                 90.0,
		P95:                 120.0,
		P99:                 200.0,
		P99_9:               300.0,
		Max:                 500.0,
		Average:             80.0,
		TargetLatency:       150.0,
		ComplianceStatus:    slo.SLOStatusHealthy,
		ViolationCount:      100,
		ViolationPercentage: 1.0,
	}
	fmt.Printf("  LatencyP95创建成功: P95=%.2fms, 目标=%.2fms, 合规状态=%s\n",
		latency.P95, latency.TargetLatency, latency.ComplianceStatus)

	// 测试SLOStatus枚举
	fmt.Printf("  SLO状态枚举: %s, %s, %s\n",
		slo.SLOStatusHealthy, slo.SLOStatusWarning, slo.SLOStatusCritical)
}

func testROITypes() {
	// 测试BaselineSnapshot
	baseline := roi.BaselineSnapshot{
		SnapshotID:        "baseline-2026-01-01",
		CPUUtilization:    25.5,
		MemUtilization:    40.2,
		TotalWasteAmount:  50000.0,
		TotalBillableCost: 200000.0,
		NodeCount:         50,
		ZombieAssetCount:  10,
		Timestamp:         time.Now(),
	}
	fmt.Printf("  BaselineSnapshot创建成功: 节点数=%d, 僵尸资产数=%d\n",
		baseline.NodeCount, baseline.ZombieAssetCount)

	// 测试CostSavingsBreakdown
	savings := roi.CostSavingsBreakdown{
		PeriodStart:                 time.Now().Add(-30 * 24 * time.Hour),
		PeriodEnd:                   time.Now(),
		ZombieCleanupSavings:        20000.0,
		ResourceOptimizationSavings: 15000.0,
		NodeConsolidationSavings:    10000.0,
		RecurringSavingsMonthly:     4500.0,
		AnnualizedSavings:           54000.0,
	}
	fmt.Printf("  CostSavingsBreakdown创建成功: 年度节省=%.2f, 月度节省=%.2f\n",
		savings.AnnualizedSavings, savings.RecurringSavingsMonthly)

	// 测试FinancialImpactAnalysis
	analysis := roi.FinancialImpactAnalysis{
		AnalysisPeriodStart:  time.Now().Add(-90 * 24 * time.Hour),
		AnalysisPeriodEnd:    time.Now(),
		CostAvoidance:        30000.0,
		CostReduction:        45000.0,
		EfficiencyGainsValue: 15000.0,
		TotalFinancialImpact: 90000.0,
		ROIPercentage:        450.0,
		PaybackPeriodMonths:  2.5,
		NetPresentValue:      85000.0,
		InternalRateOfReturn: 180.0,
	}
	fmt.Printf("  FinancialImpactAnalysis创建成功: ROI=%.1f%%, 回报期=%.1f月\n",
		analysis.ROIPercentage, analysis.PaybackPeriodMonths)
}
