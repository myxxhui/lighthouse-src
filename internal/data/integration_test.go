// Package data provides integration tests for the mock data layer.
package data

import (
	"context"
	"testing"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/k8s"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/data/prometheus"
	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

// TestMockDataIntegration tests the integration between different mock components
func TestMockDataIntegration(t *testing.T) {
	ctx := context.Background()

	// Create mock clients with consistent configuration
	configSeed := time.Now().UnixNano()

	promConfig := prometheus.DefaultMockConfig()
	promConfig.RandomSeed = configSeed
	promConfig.DataSize = "medium"
	promConfig.Scenario = "standard"

	k8sConfig := k8s.DefaultMockConfig()
	k8sConfig.RandomSeed = configSeed
	k8sConfig.DataSize = "medium"
	k8sConfig.Scenario = "standard"

	postgresConfig := postgres.DefaultMockConfig()
	postgresConfig.RandomSeed = configSeed
	postgresConfig.DataSize = "medium"
	postgresConfig.Scenario = "standard"

	// Initialize all mock components
	promClient := prometheus.NewMockClient(promConfig)
	k8sClient := k8s.NewMockClient(k8sConfig)
	postgresRepo := postgres.NewMockRepository(postgresConfig)

	// Test 1: Health checks should all pass
	t.Run("HealthChecks", func(t *testing.T) {
		if err := promClient.HealthCheck(ctx); err != nil {
			t.Errorf("Prometheus health check failed: %v", err)
		}

		if err := k8sClient.HealthCheck(ctx); err != nil {
			t.Errorf("K8s health check failed: %v", err)
		}

		if err := postgresRepo.HealthCheck(ctx); err != nil {
			t.Errorf("PostgreSQL health check failed: %v", err)
		}
	})

	// Test 2: Data consistency between Prometheus and K8s
	t.Run("DataConsistency", func(t *testing.T) {
		// Get namespaces from K8s
		namespaces, err := k8sClient.GetNamespaces(ctx)
		if err != nil {
			t.Fatalf("Failed to get namespaces: %v", err)
		}

		if len(namespaces) == 0 {
			t.Fatal("Expected non-empty namespaces")
		}

		// For each namespace, verify we can get metrics from Prometheus
		for _, ns := range namespaces {
			// Skip system namespaces for this test
			if ns.Name == "kube-system" || ns.Name == "monitoring" {
				continue
			}

			// Get deployments in this namespace
			deployments, err := k8sClient.GetDeployments(ctx, ns.Name)
			if err != nil {
				t.Errorf("Failed to get deployments for namespace %s: %v", ns.Name, err)
				continue
			}

			if len(deployments) == 0 {
				// Some namespaces might not have deployments
				continue
			}

			// Get pods for the first deployment
			pods, err := k8sClient.GetPods(ctx, ns.Name, deployments[0].Name)
			if err != nil {
				t.Errorf("Failed to get pods for deployment %s: %v", deployments[0].Name, err)
				continue
			}

			if len(pods) == 0 {
				continue
			}

			// Get metrics for the first pod from Prometheus
			startTime := time.Now().Add(-1 * time.Hour)
			endTime := time.Now()

			metrics, err := promClient.GetResourceMetrics(ctx, ns.Name, deployments[0].Name, pods[0].Name, startTime, endTime)
			if err != nil {
				t.Errorf("Failed to get metrics for pod %s: %v", pods[0].Name, err)
				continue
			}

			// Verify metrics are valid
			if len(metrics) == 0 {
				t.Errorf("Expected non-empty metrics for pod %s", pods[0].Name)
				continue
			}

			for _, metric := range metrics {
				if metric.CPURequest <= 0 || metric.MemRequest <= 0 {
					t.Errorf("Invalid resource metrics for pod %s: CPU=%f, Mem=%d",
						pods[0].Name, metric.CPURequest, metric.MemRequest)
				}
			}
		}
	})

	// Test 3: Store and retrieve cost calculations
	t.Run("CostCalculationWorkflow", func(t *testing.T) {
		// Create a sample cost calculation result
		resourceResults := []costmodel.CostResult{
			{
				CPUBillableCost:        150.0,
				CPUUsageCost:           75.0,
				CPUWasteCost:           25.0,
				CPUEfficiencyScore:     0.67,
				MemBillableCost:        200.0,
				MemUsageCost:           120.0,
				MemWasteCost:           30.0,
				MemEfficiencyScore:     0.75,
				TotalBillableCost:      350.0,
				TotalUsageCost:         195.0,
				TotalWasteCost:         55.0,
				OverallEfficiencyScore: 0.71,
				OverallGrade:           costmodel.GradeHealthy,
			},
			{
				CPUBillableCost:        300.0,
				CPUUsageCost:           50.0,
				CPUWasteCost:           100.0,
				CPUEfficiencyScore:     0.25,
				MemBillableCost:        500.0,
				MemUsageCost:           100.0,
				MemWasteCost:           150.0,
				MemEfficiencyScore:     0.30,
				TotalBillableCost:      800.0,
				TotalUsageCost:         150.0,
				TotalWasteCost:         250.0,
				OverallEfficiencyScore: 0.28,
				OverallGrade:           costmodel.GradeZombie,
			},
		}

		// Create and save cost snapshot
		snapshot := postgres.CostSnapshot{
			ID:                     "integration-test-snapshot",
			CalculationID:          "integration-test-calculation",
			Timestamp:              time.Now(),
			TimeRangeStart:         time.Now().Add(-24 * time.Hour),
			TimeRangeEnd:           time.Now(),
			ResourceResults:        resourceResults,
			AggregatedResults:      make(map[costmodel.AggregationLevel][]costmodel.AggregationResult),
			TotalBillableCost:      1150.0,
			TotalUsageCost:         345.0,
			TotalWasteCost:         305.0,
			OverallEfficiencyScore: 0.495,
			ZombieCount:            1,
			OverProvisionedCount:   0,
			HealthyCount:           1,
			RiskCount:              0,
			Metadata:               map[string]interface{}{"test": "integration"},
			CreatedAt:              time.Now(),
			UpdatedAt:              time.Now(),
		}

		// Save to PostgreSQL
		if err := postgresRepo.SaveCostSnapshot(ctx, snapshot); err != nil {
			t.Fatalf("Failed to save cost snapshot: %v", err)
		}

		// Retrieve from PostgreSQL
		retrieved, err := postgresRepo.GetCostSnapshot(ctx, "integration-test-snapshot")
		if err != nil {
			t.Fatalf("Failed to retrieve cost snapshot: %v", err)
		}

		// Verify data integrity
		if retrieved == nil {
			t.Fatal("Expected non-nil retrieved snapshot")
		}

		if retrieved.ID != snapshot.ID {
			t.Errorf("ID mismatch: expected %s, got %s", snapshot.ID, retrieved.ID)
		}

		if retrieved.TotalBillableCost != snapshot.TotalBillableCost {
			t.Errorf("TotalBillableCost mismatch: expected %f, got %f",
				snapshot.TotalBillableCost, retrieved.TotalBillableCost)
		}

		if retrieved.ZombieCount != snapshot.ZombieCount {
			t.Errorf("ZombieCount mismatch: expected %d, got %d",
				snapshot.ZombieCount, retrieved.ZombieCount)
		}

		if len(retrieved.ResourceResults) != len(snapshot.ResourceResults) {
			t.Errorf("ResourceResults count mismatch: expected %d, got %d",
				len(snapshot.ResourceResults), len(retrieved.ResourceResults))
		}

		// Verify resource result grades
		healthyCount := 0
		zombieCount := 0
		for _, result := range retrieved.ResourceResults {
			switch result.OverallGrade {
			case costmodel.GradeHealthy:
				healthyCount++
			case costmodel.GradeZombie:
				zombieCount++
			}
		}

		if healthyCount != 1 {
			t.Errorf("Expected 1 healthy resource, got %d", healthyCount)
		}
		if zombieCount != 1 {
			t.Errorf("Expected 1 zombie resource, got %d", zombieCount)
		}

		// List snapshots to verify it appears in the list
		snapshots, err := postgresRepo.ListCostSnapshots(ctx, postgres.CostSnapshotFilter{
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("Failed to list cost snapshots: %v", err)
		}

		found := false
		for _, s := range snapshots {
			if s.ID == "integration-test-snapshot" {
				found = true
				break
			}
		}

		if !found {
			t.Error("Saved snapshot not found in list")
		}

		// Clean up
		if err := postgresRepo.DeleteCostSnapshot(ctx, "integration-test-snapshot"); err != nil {
			t.Errorf("Failed to delete cost snapshot: %v", err)
		}
	})

	// Test 4: End-to-end workflow simulation
	t.Run("EndToEndWorkflow", func(t *testing.T) {
		// Simulate a realistic workflow:
		// 1. Get cluster state from K8s
		// 2. Get metrics from Prometheus
		// 3. Calculate costs (simplified)
		// 4. Store results in PostgreSQL
		// 5. Generate reports

		// Step 1: Get cluster state
		nodes, err := k8sClient.GetNodes(ctx)
		if err != nil {
			t.Fatalf("Failed to get nodes: %v", err)
		}

		namespaces, err := k8sClient.GetNamespaces(ctx)
		if err != nil {
			t.Fatalf("Failed to get namespaces: %v", err)
		}

		// Step 2: Get metrics for analysis period
		analysisStart := time.Now().Add(-7 * 24 * time.Hour) // One week ago
		analysisEnd := time.Now()

		// Get node metrics
		var allMetrics []costmodel.ResourceMetric
		for _, node := range nodes {
			nodeMetrics, err := promClient.GetNodeMetrics(ctx, node.Name, analysisStart, analysisEnd)
			if err != nil {
				t.Errorf("Failed to get metrics for node %s: %v", node.Name, err)
				continue
			}
			allMetrics = append(allMetrics, nodeMetrics...)
		}

		// Step 3: Simplified cost calculation (just verification that data is usable)
		totalCPURequest := 0.0
		totalCPUUsage := 0.0
		totalMemRequest := int64(0)
		totalMemUsage := int64(0)

		for _, metric := range allMetrics {
			totalCPURequest += metric.CPURequest
			totalCPUUsage += metric.CPUUsageP95
			totalMemRequest += metric.MemRequest
			totalMemUsage += metric.MemUsageP95
		}

		// Verify we have reasonable data
		if totalCPURequest <= 0 {
			t.Error("Total CPU request should be positive")
		}
		if totalMemRequest <= 0 {
			t.Error("Total memory request should be positive")
		}

		// Calculate overall efficiency (simplified)
		cpuEfficiency := 0.0
		if totalCPURequest > 0 {
			cpuEfficiency = totalCPUUsage / totalCPURequest
		}

		memEfficiency := 0.0
		if totalMemRequest > 0 {
			memEfficiency = float64(totalMemUsage) / float64(totalMemRequest)
		}

		overallEfficiency := (cpuEfficiency + memEfficiency) / 2

		t.Logf("Workflow simulation results:")
		t.Logf("  Nodes: %d, Namespaces: %d", len(nodes), len(namespaces))
		t.Logf("  Total CPU Request: %.2f cores", totalCPURequest)
		t.Logf("  Total CPU Usage: %.2f cores", totalCPUUsage)
		t.Logf("  Total Memory Request: %.2f GB", float64(totalMemRequest)/(1024*1024*1024))
		t.Logf("  Total Memory Usage: %.2f GB", float64(totalMemUsage)/(1024*1024*1024))
		t.Logf("  Overall Efficiency: %.2f%%", overallEfficiency*100)

		// Step 4: Store results
		snapshot := postgres.CostSnapshot{
			ID:                     "workflow-simulation-" + time.Now().Format("20060102-150405"),
			CalculationID:          "workflow-simulation",
			Timestamp:              time.Now(),
			TimeRangeStart:         analysisStart,
			TimeRangeEnd:           analysisEnd,
			ResourceResults:        []costmodel.CostResult{},
			AggregatedResults:      make(map[costmodel.AggregationLevel][]costmodel.AggregationResult),
			TotalBillableCost:      totalCPURequest*100 + float64(totalMemRequest)/(1024*1024*1024)*50, // Simplified pricing
			TotalUsageCost:         totalCPUUsage*100 + float64(totalMemUsage)/(1024*1024*1024)*50,
			TotalWasteCost:         (totalCPURequest-totalCPUUsage)*100 + float64(totalMemRequest-totalMemUsage)/(1024*1024*1024)*50,
			OverallEfficiencyScore: overallEfficiency,
			ZombieCount:            0, // Would be calculated in real scenario
			OverProvisionedCount:   0,
			HealthyCount:           len(nodes),
			RiskCount:              0,
			Metadata: map[string]interface{}{
				"workflow":       "integration-test",
				"nodes":          len(nodes),
				"namespaces":     len(namespaces),
				"cpu_efficiency": cpuEfficiency,
				"mem_efficiency": memEfficiency,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := postgresRepo.SaveCostSnapshot(ctx, snapshot); err != nil {
			t.Errorf("Failed to save workflow snapshot: %v", err)
		}

		// Step 5: Generate "report" by aggregating daily costs
		dailyCosts, err := postgresRepo.AggregateDailyNamespaceCosts(ctx,
			analysisStart, analysisEnd)
		if err != nil {
			t.Errorf("Failed to aggregate daily costs: %v", err)
		} else if len(dailyCosts) > 0 {
			t.Logf("Generated report with %d namespace aggregates", len(dailyCosts))

			totalReportedCost := 0.0
			for _, cost := range dailyCosts {
				totalReportedCost += cost.BillableCost
			}
			t.Logf("  Total reported cost: $%.2f", totalReportedCost)
		}
	})

	// Test 5: Error scenario handling
	t.Run("ErrorScenarios", func(t *testing.T) {
		// Test with error-prone configuration
		errorConfig := prometheus.DefaultMockConfig()
		errorConfig.ErrorRate = 0.5 // 50% error rate
		errorConfig.Scenario = "error"

		errorClient := prometheus.NewMockClient(errorConfig)

		startTime := time.Now().Add(-1 * time.Hour)
		endTime := time.Now()

		// Try multiple times - some should succeed, some should fail
		successCount := 0
		errorCount := 0

		for i := 0; i < 10; i++ {
			_, err := errorClient.GetResourceMetrics(ctx, "default", "test", "test", startTime, endTime)
			if err != nil {
				errorCount++
				// Verify error message contains expected text
				if err.Error() != "mock Prometheus error: simulated failure" {
					t.Errorf("Unexpected error message: %v", err)
				}
			} else {
				successCount++
			}
		}

		t.Logf("Error scenario test: %d successes, %d errors", successCount, errorCount)

		// With 50% error rate, we should see both successes and errors
		if successCount == 0 {
			t.Error("Expected at least some successes with 50% error rate")
		}
		if errorCount == 0 {
			t.Error("Expected at least some errors with 50% error rate")
		}
	})
}

// TestMockDataPerformance tests the performance characteristics of mock components
func TestMockDataPerformance(t *testing.T) {
	ctx := context.Background()

	// Create components with minimal latency
	promConfig := prometheus.DefaultMockConfig()
	promConfig.LatencyMs = 0
	promConfig.DataSize = "medium"

	k8sConfig := k8s.DefaultMockConfig()
	k8sConfig.LatencyMs = 0
	k8sConfig.DataSize = "medium"

	postgresConfig := postgres.DefaultMockConfig()
	postgresConfig.LatencyMs = 0
	postgresConfig.DataSize = "medium"

	promClient := prometheus.NewMockClient(promConfig)
	k8sClient := k8s.NewMockClient(k8sConfig)
	postgresRepo := postgres.NewMockRepository(postgresConfig)

	// Test response times for common operations
	operations := []struct {
		name string
		fn   func() error
	}{
		{
			name: "Prometheus.GetResourceMetrics",
			fn: func() error {
				_, err := promClient.GetResourceMetrics(ctx, "default", "test", "test",
					time.Now().Add(-1*time.Hour), time.Now())
				return err
			},
		},
		{
			name: "K8s.GetNamespaces",
			fn: func() error {
				_, err := k8sClient.GetNamespaces(ctx)
				return err
			},
		},
		{
			name: "K8s.GetDeployments",
			fn: func() error {
				_, err := k8sClient.GetDeployments(ctx, "default")
				return err
			},
		},
		{
			name: "PostgreSQL.ListCostSnapshots",
			fn: func() error {
				_, err := postgresRepo.ListCostSnapshots(ctx, postgres.CostSnapshotFilter{Limit: 10})
				return err
			},
		},
		{
			name: "PostgreSQL.SaveCostSnapshot",
			fn: func() error {
				snapshot := postgres.CostSnapshot{
					ID:                "perf-test",
					CalculationID:     "perf-test",
					Timestamp:         time.Now(),
					TotalBillableCost: 100.0,
					CreatedAt:         time.Now(),
					UpdatedAt:         time.Now(),
				}
				return postgresRepo.SaveCostSnapshot(ctx, snapshot)
			},
		},
	}

	// Run each operation multiple times and measure performance
	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			iterations := 10
			totalTime := time.Duration(0)

			for i := 0; i < iterations; i++ {
				start := time.Now()
				if err := op.fn(); err != nil {
					t.Errorf("Operation failed: %v", err)
					break
				}
				elapsed := time.Since(start)
				totalTime += elapsed
			}

			avgTime := totalTime / time.Duration(iterations)
			t.Logf("%s: average time %v over %d iterations", op.name, avgTime, iterations)

			// Performance check: operations should complete quickly
			// Using a generous threshold of 100ms for mock operations
			if avgTime > 100*time.Millisecond {
				t.Errorf("Operation too slow: average time %v exceeds 100ms threshold", avgTime)
			}
		})
	}

	// Test data size impact on performance
	t.Run("DataSizeImpact", func(t *testing.T) {
		sizes := []struct {
			name string
			size string
		}{
			{"Small", "small"},
			{"Medium", "medium"},
			{"Large", "large"},
		}

		for _, size := range sizes {
			t.Run(size.name, func(t *testing.T) {
				config := prometheus.DefaultMockConfig()
				config.DataSize = size.size
				config.LatencyMs = 0

				client := prometheus.NewMockClient(config)

				start := time.Now()
				metrics, err := client.GetResourceMetrics(ctx, "default", "test", "test",
					time.Now().Add(-1*time.Hour), time.Now())
				elapsed := time.Since(start)

				if err != nil {
					t.Errorf("Failed to get metrics: %v", err)
					return
				}

				t.Logf("Data size %s: %d metrics in %v", size.name, len(metrics), elapsed)

				// Verify data size matches expectation
				var expectedMin, expectedMax int
				switch size.size {
				case "small":
					expectedMin, expectedMax = 8, 12
				case "medium":
					expectedMin, expectedMax = 25, 35
				case "large":
					expectedMin, expectedMax = 90, 110
				}

				if len(metrics) < expectedMin || len(metrics) > expectedMax {
					t.Errorf("Unexpected metric count for size %s: got %d, expected %d-%d",
						size.name, len(metrics), expectedMin, expectedMax)
				}
			})
		}
	})
}

// TestMockConfiguration tests configuration options for mock components
func TestMockConfiguration(t *testing.T) {
	ctx := context.Background()

	// Test different scenarios
	scenarios := []struct {
		name     string
		scenario string
	}{
		{"Standard", "standard"},
		{"Zombie", "zombie"},
		{"Risk", "risk"},
		{"Chaos", "chaos"},
		{"Historical", "historical"},
		{"Empty", "empty"},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Configure all components with the same scenario
			promConfig := prometheus.DefaultMockConfig()
			promConfig.Scenario = scenario.scenario
			promConfig.DataSize = "small"

			k8sConfig := k8s.DefaultMockConfig()
			k8sConfig.Scenario = scenario.scenario
			k8sConfig.DataSize = "small"

			postgresConfig := postgres.DefaultMockConfig()
			postgresConfig.Scenario = scenario.scenario
			postgresConfig.DataSize = "small"

			// Initialize components
			promClient := prometheus.NewMockClient(promConfig)
			k8sClient := k8s.NewMockClient(k8sConfig)
			postgresRepo := postgres.NewMockRepository(postgresConfig)

			// Test basic functionality for each scenario
			if scenario.scenario != "empty" {
				// Prometheus should return data for non-empty scenarios
				metrics, err := promClient.GetResourceMetrics(ctx, "default", "test", "test",
					time.Now().Add(-1*time.Hour), time.Now())
				if err != nil {
					t.Errorf("Prometheus failed in %s scenario: %v", scenario.name, err)
				} else if len(metrics) == 0 && scenario.scenario != "empty" {
					t.Errorf("Expected non-empty metrics in %s scenario", scenario.name)
				}

				// K8s should return data for non-empty scenarios
				namespaces, err := k8sClient.GetNamespaces(ctx)
				if err != nil {
					t.Errorf("K8s failed in %s scenario: %v", scenario.name, err)
				} else if len(namespaces) == 0 && scenario.scenario != "empty" {
					t.Errorf("Expected non-empty namespaces in %s scenario", scenario.name)
				}

				// PostgreSQL should have data for non-empty scenarios
				snapshots, err := postgresRepo.ListCostSnapshots(ctx, postgres.CostSnapshotFilter{Limit: 5})
				if err != nil {
					t.Errorf("PostgreSQL failed in %s scenario: %v", scenario.name, err)
				} else if len(snapshots) == 0 && scenario.scenario != "empty" {
					t.Errorf("Expected non-empty snapshots in %s scenario", scenario.name)
				}
			} else {
				// Empty scenario should return empty results
				metrics, err := promClient.GetResourceMetrics(ctx, "default", "test", "test",
					time.Now().Add(-1*time.Hour), time.Now())
				if err != nil {
					t.Errorf("Prometheus failed in empty scenario: %v", err)
				} else if len(metrics) != 0 {
					t.Errorf("Expected empty metrics in empty scenario, got %d", len(metrics))
				}

				namespaces, err := k8sClient.GetNamespaces(ctx)
				if err != nil {
					t.Errorf("K8s failed in empty scenario: %v", err)
				} else if len(namespaces) != 0 {
					t.Errorf("Expected empty namespaces in empty scenario, got %d", len(namespaces))
				}
			}

			t.Logf("%s scenario: configuration test passed", scenario.name)
		})
	}
}
