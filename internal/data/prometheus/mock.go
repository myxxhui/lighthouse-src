// Package prometheus provides mock implementations for testing.
package prometheus

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

// MockConfig defines configuration options for the mock Prometheus client.
type MockConfig struct {
	// Scenario defines the test scenario to simulate
	Scenario string `json:"scenario"` // "standard", "zombie", "risk", "empty"

	// DataSize defines the size of generated data sets
	DataSize string `json:"data_size"` // "small", "medium", "large"

	// Namespaces to include in mock data
	Namespaces []string `json:"namespaces"`

	// Workloads per namespace
	WorkloadsPerNamespace int `json:"workloads_per_namespace"`

	// Pods per workload
	PodsPerWorkload int `json:"pods_per_workload"`

	// Nodes to simulate
	Nodes []string `json:"nodes"`

	// RandomSeed for deterministic generation
	RandomSeed int64 `json:"random_seed"`

	// ErrorRate controls probability of returning errors (0.0 - 1.0)
	ErrorRate float64 `json:"error_rate"`

	// LatencyMs simulates network latency in milliseconds
	LatencyMs int `json:"latency_ms"`
}

// DefaultMockConfig returns a default configuration for mock data generation.
func DefaultMockConfig() MockConfig {
	return MockConfig{
		Scenario:              "standard",
		DataSize:              "medium",
		Namespaces:            []string{"default", "kube-system", "monitoring", "app-prod", "app-staging"},
		WorkloadsPerNamespace: 3,
		PodsPerWorkload:       2,
		Nodes:                 []string{"node-1", "node-2", "node-3", "node-4"},
		RandomSeed:            42,
		ErrorRate:             0.0,
		LatencyMs:             10,
	}
}

// MockClient is a mock implementation of the Prometheus Client interface.
type MockClient struct {
	config MockConfig
	rand   *rand.Rand
}

// NewMockClient creates a new mock Prometheus client with the given configuration.
func NewMockClient(config MockConfig) *MockClient {
	if config.RandomSeed == 0 {
		config.RandomSeed = time.Now().UnixNano()
	}
	return &MockClient{
		config: config,
		rand:   rand.New(rand.NewSource(config.RandomSeed)),
	}
}

// GetResourceMetrics retrieves mock resource metrics for the given parameters.
func (m *MockClient) GetResourceMetrics(ctx context.Context, namespace, workload, pod string, startTime, endTime time.Time) ([]costmodel.ResourceMetric, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock Prometheus error: simulated failure")
	}

	// Generate metrics based on configuration
	var metrics []costmodel.ResourceMetric
	metricCount := m.getMetricCount()

	for i := 0; i < metricCount; i++ {
		metric := m.generateResourceMetric(namespace, workload, pod, startTime, endTime, i)
		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// GetNodeMetrics retrieves mock node-level metrics.
func (m *MockClient) GetNodeMetrics(ctx context.Context, nodeName string, startTime, endTime time.Time) ([]costmodel.ResourceMetric, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock Prometheus error: node metrics unavailable")
	}

	var metrics []costmodel.ResourceMetric
	metricCount := m.getMetricCount() / 2 // Fewer metrics for nodes

	for i := 0; i < metricCount; i++ {
		metric := costmodel.ResourceMetric{
			CPURequest:  m.generateCPURequest("node"),
			CPUUsageP95: m.generateCPUUsage("node"),
			MemRequest:  m.generateMemoryRequest("node"),
			MemUsageP95: m.generateMemoryUsage("node"),
			Timestamp:   m.generateTimestamp(startTime, endTime, i, metricCount),
		}
		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// GetClusterMetrics retrieves mock cluster-wide metrics.
func (m *MockClient) GetClusterMetrics(ctx context.Context, startTime, endTime time.Time) ([]costmodel.ResourceMetric, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock Prometheus error: cluster metrics unavailable")
	}

	// Cluster metrics are aggregated, return a smaller set
	var metrics []costmodel.ResourceMetric
	metricCount := 5

	for i := 0; i < metricCount; i++ {
		metric := costmodel.ResourceMetric{
			CPURequest:  m.generateCPURequest("cluster") * float64(len(m.config.Nodes)),
			CPUUsageP95: m.generateCPUUsage("cluster") * float64(len(m.config.Nodes)),
			MemRequest:  m.generateMemoryRequest("cluster") * int64(len(m.config.Nodes)),
			MemUsageP95: m.generateMemoryUsage("cluster") * int64(len(m.config.Nodes)),
			Timestamp:   m.generateTimestamp(startTime, endTime, i, metricCount),
		}
		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// GetThrottlingMetrics retrieves mock CPU throttling metrics.
func (m *MockClient) GetThrottlingMetrics(ctx context.Context, namespace, pod string, startTime, endTime time.Time) ([]ThrottlingMetric, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock Prometheus error: throttling metrics unavailable")
	}

	var metrics []ThrottlingMetric
	metricCount := m.getMetricCount() / 3

	for i := 0; i < metricCount; i++ {
		throttlingRate := m.generateThrottlingRate()
		metric := ThrottlingMetric{
			Namespace:       namespace,
			Pod:             pod,
			Container:       fmt.Sprintf("container-%d", i+1),
			ThrottledPeriod: throttlingRate * 60.0, // Assume 60-second period
			TotalPeriod:     60.0,
			ThrottlingRate:  throttlingRate * 100.0, // Convert to percentage
			Timestamp:       m.generateTimestamp(startTime, endTime, i, metricCount),
		}
		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// GetSaturationMetrics retrieves mock resource saturation metrics.
func (m *MockClient) GetSaturationMetrics(ctx context.Context, resourceType string, startTime, endTime time.Time) ([]SaturationMetric, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock Prometheus error: saturation metrics unavailable")
	}

	var metrics []SaturationMetric
	metricCount := m.getMetricCount() / 2

	for i := 0; i < metricCount; i++ {
		nodeIdx := i % len(m.config.Nodes)
		metric := SaturationMetric{
			ResourceType: resourceType,
			Node:         m.config.Nodes[nodeIdx],
			Saturation:   m.generateSaturation(),
			Timestamp:    m.generateTimestamp(startTime, endTime, i, metricCount),
		}
		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// HealthCheck always returns nil (healthy) for mock client.
func (m *MockClient) HealthCheck(ctx context.Context) error {
	if m.shouldReturnError() {
		return fmt.Errorf("mock Prometheus health check failed")
	}
	return nil
}

// Helper methods

func (m *MockClient) simulateLatency() error {
	if m.config.LatencyMs > 0 {
		time.Sleep(time.Duration(m.config.LatencyMs) * time.Millisecond)
	}
	return nil
}

func (m *MockClient) shouldReturnError() bool {
	if m.config.ErrorRate <= 0.0 {
		return false
	}
	return m.rand.Float64() < m.config.ErrorRate
}

func (m *MockClient) getMetricCount() int {
	switch m.config.DataSize {
	case "small":
		return 10
	case "large":
		return 100
	default: // "medium"
		return 30
	}
}

func (m *MockClient) generateResourceMetric(namespace, workload, pod string, startTime, endTime time.Time, index int) costmodel.ResourceMetric {
	cpuRequest := m.generateCPURequest("pod")
	cpuUsage := m.generateCPUUsage("pod")
	memRequest := m.generateMemoryRequest("pod")
	memUsage := m.generateMemoryUsage("pod")

	return costmodel.ResourceMetric{
		CPURequest:  cpuRequest,
		CPUUsageP95: cpuUsage,
		MemRequest:  memRequest,
		MemUsageP95: memUsage,
		Timestamp:   m.generateTimestamp(startTime, endTime, index, m.getMetricCount()),
	}
}

func (m *MockClient) generateCPURequest(resourceType string) float64 {
	// Base values by resource type and scenario
	var base, variation float64

	switch resourceType {
	case "pod":
		base = 0.5
		variation = 4.0
	case "node":
		base = 8.0
		variation = 16.0
	case "cluster":
		base = 32.0
		variation = 64.0
	default:
		base = 1.0
		variation = 2.0
	}

	// Adjust based on scenario
	switch m.config.Scenario {
	case "zombie":
		base *= 5.0 // Over-provisioned
	case "risk":
		base *= 0.8 // Under-provisioned
	}

	return base + m.rand.Float64()*variation
}

func (m *MockClient) generateCPUUsage(resourceType string) float64 {
	request := m.generateCPURequest(resourceType)

	// Usage as percentage of request, based on scenario
	var usageRatio float64
	switch m.config.Scenario {
	case "standard":
		usageRatio = 0.3 + m.rand.Float64()*0.4 // 30-70%
	case "zombie":
		usageRatio = 0.05 + m.rand.Float64()*0.1 // 5-15%
	case "risk":
		usageRatio = 0.85 + m.rand.Float64()*0.15 // 85-100%
	case "empty":
		return 0.0
	default:
		usageRatio = 0.5
	}

	return request * usageRatio
}

func (m *MockClient) generateMemoryRequest(resourceType string) int64 {
	// Base values in bytes
	var baseGB, variationGB float64

	switch resourceType {
	case "pod":
		baseGB = 1.0
		variationGB = 3.0
	case "node":
		baseGB = 16.0
		variationGB = 32.0
	case "cluster":
		baseGB = 64.0
		variationGB = 128.0
	default:
		baseGB = 2.0
		variationGB = 4.0
	}

	// Adjust based on scenario
	switch m.config.Scenario {
	case "zombie":
		baseGB *= 4.0 // Over-provisioned
	case "risk":
		baseGB *= 0.7 // Under-provisioned
	}

	gb := baseGB + m.rand.Float64()*variationGB
	return int64(gb * 1024 * 1024 * 1024) // Convert to bytes
}

func (m *MockClient) generateMemoryUsage(resourceType string) int64 {
	request := m.generateMemoryRequest(resourceType)

	// Usage as percentage of request, similar to CPU
	var usageRatio float64
	switch m.config.Scenario {
	case "standard":
		usageRatio = 0.25 + m.rand.Float64()*0.5 // 25-75%
	case "zombie":
		usageRatio = 0.08 + m.rand.Float64()*0.12 // 8-20%
	case "risk":
		usageRatio = 0.9 + m.rand.Float64()*0.1 // 90-100%
	case "empty":
		return 0
	default:
		usageRatio = 0.5
	}

	return int64(float64(request) * usageRatio)
}

func (m *MockClient) generateTimestamp(startTime, endTime time.Time, index, total int) time.Time {
	if startTime.IsZero() || endTime.IsZero() || startTime.Equal(endTime) {
		// Default to recent time if not specified
		now := time.Now()
		return now.Add(-time.Duration(total-index) * time.Hour)
	}

	// Distribute timestamps evenly across the time range
	duration := endTime.Sub(startTime)
	interval := duration / time.Duration(total)
	return startTime.Add(interval * time.Duration(index))
}

func (m *MockClient) generateThrottlingRate() float64 {
	switch m.config.Scenario {
	case "standard":
		return 0.01 + m.rand.Float64()*0.05 // 1-6%
	case "risk":
		return 0.1 + m.rand.Float64()*0.2 // 10-30%
	case "zombie":
		return 0.0 // No throttling for zombie pods
	default:
		return 0.02
	}
}

func (m *MockClient) generateSaturation() float64 {
	switch m.config.Scenario {
	case "standard":
		return 40.0 + m.rand.Float64()*30.0 // 40-70%
	case "risk":
		return 85.0 + m.rand.Float64()*15.0 // 85-100%
	case "zombie":
		return 10.0 + m.rand.Float64()*20.0 // 10-30%
	default:
		return 50.0
	}
}
