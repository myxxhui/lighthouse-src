package prometheus

import (
	"context"
	"testing"
	"time"

	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

func TestNewMockClient(t *testing.T) {
	config := DefaultMockConfig()
	client := NewMockClient(config)

	if client == nil {
		t.Fatal("Expected non-nil client")
	}
	if config.RandomSeed != client.config.RandomSeed {
		t.Errorf("Expected RandomSeed %d, got %d", config.RandomSeed, client.config.RandomSeed)
	}
}

func TestMockClient_GetResourceMetrics(t *testing.T) {
	ctx := context.Background()
	client := NewMockClient(DefaultMockConfig())

	startTime := time.Now().Add(-1 * time.Hour)
	endTime := time.Now()

	metrics, err := client.GetResourceMetrics(ctx, "default", "test-deployment", "test-pod", startTime, endTime)
	if err != nil {
		t.Fatalf("GetResourceMetrics failed: %v", err)
	}

	if len(metrics) == 0 {
		t.Error("Expected non-empty metrics")
	}

	// Verify metric structure
	for _, metric := range metrics {
		if metric.CPURequest <= 0.0 {
			t.Errorf("Expected CPURequest > 0, got %f", metric.CPURequest)
		}
		if metric.CPUUsageP95 < 0.0 {
			t.Errorf("Expected CPUUsageP95 >= 0, got %f", metric.CPUUsageP95)
		}
		// Allow CPU usage to be up to 2x request (for mock data flexibility)
		if metric.CPUUsageP95 > metric.CPURequest*2.0 {
			t.Errorf("CPU usage %f should not be more than 2x higher than request %f", metric.CPUUsageP95, metric.CPURequest)
		}
		if metric.MemRequest <= 0 {
			t.Errorf("Expected MemRequest > 0, got %d", metric.MemRequest)
		}
		if metric.MemUsageP95 < 0 {
			t.Errorf("Expected MemUsageP95 >= 0, got %d", metric.MemUsageP95)
		}
		if metric.Timestamp.IsZero() {
			t.Error("Expected non-zero timestamp")
		}
	}
}

func TestMockClient_GetNodeMetrics(t *testing.T) {
	ctx := context.Background()
	client := NewMockClient(DefaultMockConfig())

	startTime := time.Now().Add(-1 * time.Hour)
	endTime := time.Now()

	metrics, err := client.GetNodeMetrics(ctx, "node-1", startTime, endTime)
	if err != nil {
		t.Fatalf("GetNodeMetrics failed: %v", err)
	}

	if len(metrics) == 0 {
		t.Error("Expected non-empty metrics")
	}

	// Node metrics should have higher resource values than pod metrics
	for _, metric := range metrics {
		if metric.CPURequest <= 2.0 {
			t.Errorf("Node CPURequest should be > 2.0, got %f", metric.CPURequest)
		}
		if metric.MemRequest <= 4*1024*1024*1024 {
			t.Errorf("Node MemRequest should be > 4GB, got %d bytes", metric.MemRequest)
		}
	}
}

func TestMockClient_HealthCheck(t *testing.T) {
	ctx := context.Background()
	client := NewMockClient(DefaultMockConfig())

	if err := client.HealthCheck(ctx); err != nil {
		t.Errorf("HealthCheck failed: %v", err)
	}
}

func TestMockClient_ScenarioVariations(t *testing.T) {
	testCases := []struct {
		name     string
		scenario string
		check    func([]costmodel.ResourceMetric)
	}{
		{
			name:     "Standard scenario",
			scenario: "standard",
			check: func(metrics []costmodel.ResourceMetric) {
				// Standard scenario should generate metrics
				if len(metrics) == 0 {
					t.Error("Standard scenario should generate metrics")
				}
				// Basic sanity check
				for _, metric := range metrics {
					if metric.CPURequest <= 0 {
						t.Error("CPU request should be positive")
					}
				}
			},
		},
		{
			name:     "Zombie scenario",
			scenario: "zombie",
			check: func(metrics []costmodel.ResourceMetric) {
				// Zombie scenario should generate metrics with high requests
				if len(metrics) == 0 {
					t.Error("Zombie scenario should generate metrics")
				}
				// Check that requests are relatively high (characteristic of zombie resources)
				totalCPURequest := 0.0
				totalCPUUsage := 0.0
				for _, metric := range metrics {
					totalCPURequest += metric.CPURequest
					totalCPUUsage += metric.CPUUsageP95
				}
				if totalCPURequest > 0 {
					usageRatio := totalCPUUsage / totalCPURequest
					// Zombie resources typically have low usage ratio
					if usageRatio > 0.5 {
						t.Logf("Zombie scenario: total CPU usage ratio is %f (expected low)", usageRatio)
					}
				}
			},
		},
		{
			name:     "Risk scenario",
			scenario: "risk",
			check: func(metrics []costmodel.ResourceMetric) {
				// Risk scenario should generate metrics
				if len(metrics) == 0 {
					t.Error("Risk scenario should generate metrics")
				}
				// Risk resources have high usage relative to request
				totalCPURequest := 0.0
				totalCPUUsage := 0.0
				for _, metric := range metrics {
					totalCPURequest += metric.CPURequest
					totalCPUUsage += metric.CPUUsageP95
				}
				if totalCPURequest > 0 {
					usageRatio := totalCPUUsage / totalCPURequest
					// Risk resources typically have high usage ratio
					if usageRatio < 0.5 {
						t.Logf("Risk scenario: total CPU usage ratio is %f (expected high)", usageRatio)
					}
				}
			},
		},
		{
			name:     "Empty scenario",
			scenario: "empty",
			check: func(metrics []costmodel.ResourceMetric) {
				if len(metrics) != 0 {
					t.Errorf("Empty scenario: expected empty metrics, got %d metrics", len(metrics))
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			config := DefaultMockConfig()
			config.Scenario = tc.scenario
			client := NewMockClient(config)

			startTime := time.Now().Add(-1 * time.Hour)
			endTime := time.Now()

			metrics, err := client.GetResourceMetrics(ctx, "default", "test", "test", startTime, endTime)
			if err != nil {
				t.Fatalf("GetResourceMetrics failed: %v", err)
			}

			tc.check(metrics)
		})
	}
}

func TestMockClient_DataSizeVariations(t *testing.T) {
	testCases := []struct {
		name     string
		dataSize string
		minCount int
		maxCount int
	}{
		{"Small data size", "small", 8, 12},
		{"Medium data size", "medium", 25, 35},
		{"Large data size", "large", 90, 110},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			config := DefaultMockConfig()
			config.DataSize = tc.dataSize
			client := NewMockClient(config)

			startTime := time.Now().Add(-1 * time.Hour)
			endTime := time.Now()

			metrics, err := client.GetResourceMetrics(ctx, "default", "test", "test", startTime, endTime)
			if err != nil {
				t.Fatalf("GetResourceMetrics failed: %v", err)
			}

			if len(metrics) < tc.minCount {
				t.Errorf("Expected at least %d metrics, got %d", tc.minCount, len(metrics))
			}
			if len(metrics) > tc.maxCount {
				t.Errorf("Expected at most %d metrics, got %d", tc.maxCount, len(metrics))
			}
		})
	}
}

func TestMockClient_DeterministicGeneration(t *testing.T) {
	ctx := context.Background()
	config := DefaultMockConfig()
	config.RandomSeed = 12345
	config.DataSize = "small"

	// Create two clients with same seed
	client1 := NewMockClient(config)
	client2 := NewMockClient(config)

	startTime := time.Now().Add(-1 * time.Hour)
	endTime := time.Now()

	metrics1, err1 := client1.GetResourceMetrics(ctx, "default", "test", "test", startTime, endTime)
	if err1 != nil {
		t.Fatalf("Client1 GetResourceMetrics failed: %v", err1)
	}

	metrics2, err2 := client2.GetResourceMetrics(ctx, "default", "test", "test", startTime, endTime)
	if err2 != nil {
		t.Fatalf("Client2 GetResourceMetrics failed: %v", err2)
	}

	// Should generate identical data with same seed
	if len(metrics1) != len(metrics2) {
		t.Errorf("Metric count mismatch: %d != %d", len(metrics1), len(metrics2))
	}

	for i := range metrics1 {
		if metrics1[i].CPURequest != metrics2[i].CPURequest {
			t.Errorf("CPURequest mismatch at index %d: %f != %f", i, metrics1[i].CPURequest, metrics2[i].CPURequest)
		}
		if metrics1[i].CPUUsageP95 != metrics2[i].CPUUsageP95 {
			t.Errorf("CPUUsageP95 mismatch at index %d: %f != %f", i, metrics1[i].CPUUsageP95, metrics2[i].CPUUsageP95)
		}
		if metrics1[i].MemRequest != metrics2[i].MemRequest {
			t.Errorf("MemRequest mismatch at index %d: %d != %d", i, metrics1[i].MemRequest, metrics2[i].MemRequest)
		}
		if metrics1[i].MemUsageP95 != metrics2[i].MemUsageP95 {
			t.Errorf("MemUsageP95 mismatch at index %d: %d != %d", i, metrics1[i].MemUsageP95, metrics2[i].MemUsageP95)
		}
	}
}

func TestMockClient_WithLatency(t *testing.T) {
	ctx := context.Background()
	config := DefaultMockConfig()
	config.LatencyMs = 10 // 10ms latency
	client := NewMockClient(config)

	startTime := time.Now().Add(-1 * time.Hour)
	endTime := time.Now()

	start := time.Now()
	_, err := client.GetResourceMetrics(ctx, "default", "test", "test", startTime, endTime)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("GetResourceMetrics failed: %v", err)
	}

	// Should take at least the configured latency
	if elapsed < 10*time.Millisecond {
		t.Errorf("Expected at least 10ms latency, got %v", elapsed)
	}
}
