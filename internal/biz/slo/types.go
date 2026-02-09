// Package slo defines the business domain types for SLO (Service Level Objective) monitoring and diagnostics.
// These types support dynamic SLO monitoring, fault diagnosis, and evidence chain collection.
package slo

import (
	"time"
)

// SLOStatus represents the current status of a Service Level Objective.
type SLOStatus string

const (
	SLOStatusHealthy  SLOStatus = "healthy"  // Green light - SLO is met
	SLOStatusWarning  SLOStatus = "warning"  // Yellow light - SLO is degraded
	SLOStatusCritical SLOStatus = "critical" // Red light - SLO is violated
)

// SLOMetrics represents the key metrics used for SLO calculation.
type SLOMetrics struct {
	// Availability metrics
	TotalRequests      int64   `json:"total_requests"`
	SuccessfulRequests int64   `json:"successful_requests"`
	AvailabilityRate   float64 `json:"availability_rate"` // Success rate percentage

	// Latency metrics (in milliseconds)
	LatencyP95     float64 `json:"latency_p95"`
	LatencyP99     float64 `json:"latency_p99"`
	AverageLatency float64 `json:"average_latency"`

	// Error metrics
	ErrorCount int64   `json:"error_count"`
	ErrorRate  float64 `json:"error_rate"`

	// Timestamp of the metrics
	Timestamp time.Time `json:"timestamp"`
}

// SLOConfig represents the configuration for SLO monitoring.
type SLOConfig struct {
	// SLO thresholds
	AvailabilityThreshold float64 `json:"availability_threshold"` // e.g., 99.9 for 99.9%
	LatencyP95Threshold   float64 `json:"latency_p95_threshold"`  // e.g., 500.0 for 500ms

	// Aggregation level
	AggregationLevel string `json:"aggregation_level"` // global, namespace, service

	// Identifier (namespace name, service name, etc.)
	Identifier string `json:"identifier"`

	// Evaluation window (in minutes)
	EvaluationWindow int `json:"evaluation_window"`
}

// SLOResult represents the result of SLO evaluation.
type SLOResult struct {
	// Configuration used for evaluation
	Config SLOConfig `json:"config"`

	// Metrics used for evaluation
	Metrics SLOMetrics `json:"metrics"`

	// Evaluation result
	Status SLOStatus `json:"status"`

	// Violation details (if status is critical or warning)
	ViolationDetails *SLOViolationDetails `json:"violation_details,omitempty"`

	// Evaluation timestamp
	EvaluatedAt time.Time `json:"evaluated_at"`
}

// SLOViolationDetails provides detailed information about SLO violations.
type SLOViolationDetails struct {
	// Violation type
	ViolationType string `json:"violation_type"` // availability, latency, error_rate

	// Actual vs threshold values
	ActualValue    float64 `json:"actual_value"`
	ThresholdValue float64 `json:"threshold_value"`

	// Top failing endpoints (if applicable)
	TopFailingEndpoints []string `json:"top_failing_endpoints"`

	// Error code distribution (if applicable)
	ErrorCodeDistribution map[string]int `json:"error_code_distribution"`
}

// SnapshotTrigger represents the conditions that trigger a contextual snapshot.
type SnapshotTrigger struct {
	// Trigger condition
	Condition string `json:"condition"` // e.g., "slo_violation", "manual_trigger"

	// SLO violation that triggered the snapshot
	SLOViolation *SLOResult `json:"slo_violation,omitempty"`

	// Time range for snapshot collection
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	// Trigger timestamp
	TriggeredAt time.Time `json:"triggered_at"`
}

// EvidenceChain represents the complete evidence chain collected during a snapshot.
// This includes impact, change, and resource dimensions.
type EvidenceChain struct {
	// Snapshot metadata
	SnapshotID string          `json:"snapshot_id"`
	Trigger    SnapshotTrigger `json:"trigger"`

	// Impact dimension (user impact)
	Impact EvidenceImpact `json:"impact"`

	// Change dimension (recent changes)
	Change EvidenceChange `json:"change"`

	// Resource dimension (resource metrics)
	Resource EvidenceResource `json:"resource"`

	// Collection timestamp
	CollectedAt time.Time `json:"collected_at"`
}

// EvidenceImpact represents the user impact dimension of the evidence chain.
type EvidenceImpact struct {
	// Affected users
	AffectedUVCount int64 `json:"affected_uv_count"`

	// Top failing interfaces
	TopFailingInterfaces []string `json:"top_failing_interfaces"`

	// Error code distribution
	ErrorCodeDistribution map[string]int `json:"error_code_distribution"`

	// Geographic impact (if available)
	GeographicImpact map[string]int `json:"geographic_impact"`

	// ISP/Carrier impact (if available)
	ISPImpact map[string]int `json:"isp_impact"`
}

// EvidenceChange represents the change dimension of the evidence chain.
type EvidenceChange struct {
	// K8s events in the time window
	K8sEvents []K8sEvent `json:"k8s_events"`

	// Configuration changes
	ConfigChanges []ConfigChange `json:"config_changes"`

	// Anomaly events
	AnomalyEvents []AnomalyEvent `json:"anomaly_events"`
}

// K8sEvent represents a Kubernetes event in the evidence chain.
type K8sEvent struct {
	// Event type
	Type string `json:"type"` // ImageUpdate, Scaling, ConfigMapChange, etc.

	// Object details
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // Pod, Deployment, Service, etc.

	// Event message
	Message string `json:"message"`

	// Event timestamp
	Timestamp time.Time `json:"timestamp"`
}

// ConfigChange represents a configuration change in the evidence chain.
type ConfigChange struct {
	// Change type
	ChangeType string `json:"change_type"` // Helm deployment, CRD update, etc.

	// Resource details
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`

	// Change details
	OldValue string `json:"old_value"`
	NewValue string `json:"new_value"`

	// Change timestamp
	Timestamp time.Time `json:"timestamp"`
}

// AnomalyEvent represents an anomaly event in the evidence chain.
type AnomalyEvent struct {
	// Event type
	EventType string `json:"event_type"` // OOMKilled, LivenessProbeFailed, NodeNotReady, etc.

	// Resource details
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`

	// Event details
	Details string `json:"details"`

	// Event timestamp
	Timestamp time.Time `json:"timestamp"`
}

// EvidenceResource represents the resource dimension of the evidence chain.
type EvidenceResource struct {
	// CPU throttling metrics
	CPUThrottling []ResourceMetric `json:"cpu_throttling"`

	// Memory usage metrics
	MemoryUsage []ResourceMetric `json:"memory_usage"`

	// Node metrics
	NodeMetrics []NodeMetric `json:"node_metrics"`

	// Dependency metrics
	DependencyMetrics []DependencyMetric `json:"dependency_metrics"`
}

// ResourceMetric represents a time-series resource metric.
type ResourceMetric struct {
	// Resource identifier
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`

	// Metric type
	MetricType string `json:"metric_type"` // cpu_throttling, memory_usage, etc.

	// Metric values over time
	Values []MetricValue `json:"values"`
}

// MetricValue represents a single point in a time-series metric.
type MetricValue struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// NodeMetric represents node-level metrics in the evidence chain.
type NodeMetric struct {
	// Node identifier
	NodeName string `json:"node_name"`

	// Load metrics
	LoadAverage float64 `json:"load_average"`

	// Disk I/O metrics
	DiskIORead  float64 `json:"disk_io_read"`
	DiskIOWrite float64 `json:"disk_io_write"`

	// Network metrics
	NetworkCongestion float64 `json:"network_congestion"`

	// Timestamp
	Timestamp time.Time `json:"timestamp"`
}

// DependencyMetric represents metrics for dependent services.
type DependencyMetric struct {
	// Dependency identifier
	ServiceName string `json:"service_name"`

	// Connection pool metrics
	DBConnectionPoolSize int `json:"db_connection_pool_size"`
	DBConnectionPoolUsed int `json:"db_connection_pool_used"`

	// Latency metrics
	DependencyLatencyP95 float64 `json:"dependency_latency_p95"`

	// Error metrics
	DependencyErrorRate float64 `json:"dependency_error_rate"`

	// Timestamp
	Timestamp time.Time `json:"timestamp"`
}
