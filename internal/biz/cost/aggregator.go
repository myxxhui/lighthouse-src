// Package cost defines the business domain types and interfaces for cost calculation and resource analysis.
// This file contains the implementation of Aggregator interfaces for different aggregation levels.
package cost

import (
	"time"

	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

// =============================================
// Base Aggregator Implementation
// =============================================

// BaseAggregator provides common functionality for all aggregators.
type BaseAggregator struct {
	level      costmodel.AggregationLevel
	identifier string
	dimensions []string
}

// Level returns the aggregation level.
func (ba *BaseAggregator) Level() costmodel.AggregationLevel {
	return ba.level
}

// SupportsDimension checks if this aggregator supports the given dimension.
func (ba *BaseAggregator) SupportsDimension(dimension string) bool {
	for _, d := range ba.dimensions {
		if d == dimension {
			return true
		}
	}
	return false
}

// =============================================
// L1: Namespace Aggregator
// =============================================

// NamespaceAggregator implements Aggregator for namespace-level (L1) aggregation.
type NamespaceAggregator struct {
	BaseAggregator
	namespace string
}

// NewNamespaceAggregator creates a new namespace aggregator.
func NewNamespaceAggregator(namespace string) *NamespaceAggregator {
	return &NamespaceAggregator{
		BaseAggregator: BaseAggregator{
			level:      costmodel.LevelNamespace,
			identifier: namespace,
			dimensions: []string{"cost", "efficiency", "waste", "resource_count"},
		},
		namespace: namespace,
	}
}

// Aggregate performs namespace-level aggregation.
func (na *NamespaceAggregator) Aggregate(results []costmodel.DualCostResult) (*costmodel.AggregationResult, error) {
	// For type definition purposes only - implementation will be added later
	// This satisfies the interface requirement without actual logic
	return nil, nil
}

// =============================================
// L2: Node Aggregator
// =============================================

// NodeAggregator implements Aggregator for node-level (L2) aggregation.
type NodeAggregator struct {
	BaseAggregator
	nodeName string
}

// NewNodeAggregator creates a new node aggregator.
func NewNodeAggregator(nodeName string) *NodeAggregator {
	return &NodeAggregator{
		BaseAggregator: BaseAggregator{
			level:      costmodel.LevelNode,
			identifier: nodeName,
			dimensions: []string{"cost", "efficiency", "waste", "resource_allocation", "node_utilization"},
		},
		nodeName: nodeName,
	}
}

// Aggregate performs node-level aggregation.
func (na *NodeAggregator) Aggregate(results []costmodel.DualCostResult) (*costmodel.AggregationResult, error) {
	// For type definition purposes only - implementation will be added later
	return nil, nil
}

// =============================================
// L3: Workload Aggregator
// =============================================

// WorkloadAggregator implements Aggregator for workload-level (L3) aggregation.
type WorkloadAggregator struct {
	BaseAggregator
	namespace    string
	workloadName string
	workloadType string
}

// NewWorkloadAggregator creates a new workload aggregator.
func NewWorkloadAggregator(namespace, workloadName, workloadType string) *WorkloadAggregator {
	identifier := namespace + "/" + workloadName
	return &WorkloadAggregator{
		BaseAggregator: BaseAggregator{
			level:      costmodel.LevelWorkload,
			identifier: identifier,
			dimensions: []string{"cost", "efficiency", "waste", "replica_efficiency", "workload_pattern"},
		},
		namespace:    namespace,
		workloadName: workloadName,
		workloadType: workloadType,
	}
}

// Aggregate performs workload-level aggregation.
func (wa *WorkloadAggregator) Aggregate(results []costmodel.DualCostResult) (*costmodel.AggregationResult, error) {
	// For type definition purposes only - implementation will be added later
	return nil, nil
}

// =============================================
// L4: Pod Aggregator
// =============================================

// PodAggregator implements Aggregator for pod-level (L4) aggregation.
type PodAggregator struct {
	BaseAggregator
	namespace    string
	podName      string
	workloadName string
	nodeName     string
}

// NewPodAggregator creates a new pod aggregator.
func NewPodAggregator(namespace, podName, workloadName, nodeName string) *PodAggregator {
	identifier := namespace + "/" + podName
	return &PodAggregator{
		BaseAggregator: BaseAggregator{
			level:      costmodel.LevelPod,
			identifier: identifier,
			dimensions: []string{"cost", "efficiency", "waste", "container_details", "resource_requests"},
		},
		namespace:    namespace,
		podName:      podName,
		workloadName: workloadName,
		nodeName:     nodeName,
	}
}

// Aggregate performs pod-level aggregation.
func (pa *PodAggregator) Aggregate(results []costmodel.DualCostResult) (*costmodel.AggregationResult, error) {
	// For type definition purposes only - implementation will be added later
	return nil, nil
}

// =============================================
// L0: Cluster Aggregator
// =============================================

// ClusterAggregator implements Aggregator for cluster-level (L0) aggregation.
type ClusterAggregator struct {
	BaseAggregator
	clusterName string
}

// NewClusterAggregator creates a new cluster aggregator.
func NewClusterAggregator(clusterName string) *ClusterAggregator {
	return &ClusterAggregator{
		BaseAggregator: BaseAggregator{
			level:      costmodel.LevelCluster,
			identifier: clusterName,
			dimensions: []string{"cost", "efficiency", "waste", "global_metrics", "cluster_health"},
		},
		clusterName: clusterName,
	}
}

// Aggregate performs cluster-level aggregation.
func (ca *ClusterAggregator) Aggregate(results []costmodel.DualCostResult) (*costmodel.AggregationResult, error) {
	// For type definition purposes only - implementation will be added later
	return nil, nil
}

// =============================================
// Aggregator Factory
// =============================================

// AggregatorFactory creates aggregators based on level and identifier.
type AggregatorFactory struct{}

// CreateAggregator creates an appropriate aggregator for the given level and identifier.
func (af *AggregatorFactory) CreateAggregator(level costmodel.AggregationLevel, identifier string, metadata map[string]string) costmodel.Aggregator {
	switch level {
	case costmodel.LevelNamespace:
		return NewNamespaceAggregator(identifier)
	case costmodel.LevelNode:
		return NewNodeAggregator(identifier)
	case costmodel.LevelWorkload:
		namespace := metadata["namespace"]
		workloadType := metadata["workload_type"]
		return NewWorkloadAggregator(namespace, identifier, workloadType)
	case costmodel.LevelPod:
		namespace := metadata["namespace"]
		workloadName := metadata["workload_name"]
		nodeName := metadata["node_name"]
		return NewPodAggregator(namespace, identifier, workloadName, nodeName)
	case costmodel.LevelCluster:
		return NewClusterAggregator(identifier)
	default:
		// Default to cluster aggregator for unknown levels
		return NewClusterAggregator("default")
	}
}

// =============================================
// Aggregation Context
// =============================================

// AggregationContext holds the context for aggregation operations.
type AggregationContext struct {
	Aggregator costmodel.Aggregator
	Results    []costmodel.DualCostResult
	Timestamp  time.Time
	Precision  costmodel.PrecisionConfig
	Metadata   map[string]interface{}
}

// NewAggregationContext creates a new aggregation context.
func NewAggregationContext(aggregator costmodel.Aggregator, results []costmodel.DualCostResult) *AggregationContext {
	return &AggregationContext{
		Aggregator: aggregator,
		Results:    results,
		Timestamp:  time.Now(),
		Precision:  costmodel.DefaultPrecisionConfig(),
		Metadata:   make(map[string]interface{}),
	}
}

// Execute performs aggregation using the configured aggregator.
func (ctx *AggregationContext) Execute() (*costmodel.AggregationResult, error) {
	return ctx.Aggregator.Aggregate(ctx.Results)
}
