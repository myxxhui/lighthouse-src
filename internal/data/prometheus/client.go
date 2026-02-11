// Package prometheus provides client implementations for querying Prometheus metrics.
package prometheus

import (
	"context"
	"time"

	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

// Client defines the interface for Prometheus clients.
type Client interface {
	// GetResourceMetrics retrieves resource metrics (CPU/Memory Request/Usage) for the given time range.
	GetResourceMetrics(ctx context.Context, namespace, workload, pod string, startTime, endTime time.Time) ([]costmodel.ResourceMetric, error)

	// GetNodeMetrics retrieves node-level resource metrics.
	GetNodeMetrics(ctx context.Context, nodeName string, startTime, endTime time.Time) ([]costmodel.ResourceMetric, error)

	// GetClusterMetrics retrieves cluster-wide aggregated metrics.
	GetClusterMetrics(ctx context.Context, startTime, endTime time.Time) ([]costmodel.ResourceMetric, error)

	// GetThrottlingMetrics retrieves CPU throttling metrics for containers.
	GetThrottlingMetrics(ctx context.Context, namespace, pod string, startTime, endTime time.Time) ([]ThrottlingMetric, error)

	// GetSaturationMetrics retrieves resource saturation metrics.
	GetSaturationMetrics(ctx context.Context, resourceType string, startTime, endTime time.Time) ([]SaturationMetric, error)

	// HealthCheck checks if Prometheus is reachable and healthy.
	HealthCheck(ctx context.Context) error
}

// ThrottlingMetric represents CPU throttling metrics.
type ThrottlingMetric struct {
	Namespace       string    `json:"namespace"`
	Pod             string    `json:"pod"`
	Container       string    `json:"container"`
	ThrottledPeriod float64   `json:"throttled_period"` // seconds
	TotalPeriod     float64   `json:"total_period"`     // seconds
	ThrottlingRate  float64   `json:"throttling_rate"`  // percentage
	Timestamp       time.Time `json:"timestamp"`
}

// SaturationMetric represents resource saturation metrics.
type SaturationMetric struct {
	ResourceType string    `json:"resource_type"`
	Node         string    `json:"node"`
	Saturation   float64   `json:"saturation"` // 0-100 percentage
	Timestamp    time.Time `json:"timestamp"`
}
