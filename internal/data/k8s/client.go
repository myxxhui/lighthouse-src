// Package k8s provides client implementations for interacting with Kubernetes API.
package k8s

import (
	"context"
	"time"
)

// Client defines the interface for Kubernetes API clients.
type Client interface {
	// GetNamespaces retrieves list of namespaces.
	GetNamespaces(ctx context.Context) ([]Namespace, error)

	// GetDeployments retrieves deployments for a namespace.
	GetDeployments(ctx context.Context, namespace string) ([]Deployment, error)

	// GetPods retrieves pods for a namespace or deployment.
	GetPods(ctx context.Context, namespace, deployment string) ([]Pod, error)

	// GetNodes retrieves cluster nodes.
	GetNodes(ctx context.Context) ([]Node, error)

	// GetEvents retrieves events for a namespace or resource.
	GetEvents(ctx context.Context, namespace, resourceType, resourceName string) ([]Event, error)

	// GetResourceQuotas retrieves resource quotas for a namespace.
	GetResourceQuotas(ctx context.Context, namespace string) ([]ResourceQuota, error)

	// HealthCheck checks if Kubernetes API is reachable.
	HealthCheck(ctx context.Context) error
}

// Namespace represents a Kubernetes namespace.
type Namespace struct {
	Name              string            `json:"name"`
	CreationTimestamp time.Time         `json:"creation_timestamp"`
	Labels            map[string]string `json:"labels"`
	Annotations       map[string]string `json:"annotations"`
	Status            string            `json:"status"` // Active, Terminating
}

// Deployment represents a Kubernetes deployment.
type Deployment struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Replicas          int32             `json:"replicas"`
	AvailableReplicas int32             `json:"available_replicas"`
	Labels            map[string]string `json:"labels"`
	Annotations       map[string]string `json:"annotations"`
	CreationTimestamp time.Time         `json:"creation_timestamp"`
	StrategyType      string            `json:"strategy_type"` // RollingUpdate, Recreate
}

// Pod represents a Kubernetes pod.
type Pod struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Deployment        string            `json:"deployment"`
	NodeName          string            `json:"node_name"`
	Phase             string            `json:"phase"` // Running, Pending, Succeeded, Failed
	CreationTimestamp time.Time         `json:"creation_timestamp"`
	Labels            map[string]string `json:"labels"`
	Annotations       map[string]string `json:"annotations"`
	Containers        []Container       `json:"containers"`
}

// Container represents a container within a pod.
type Container struct {
	Name      string             `json:"name"`
	Image     string             `json:"image"`
	Resources ContainerResources `json:"resources"`
	Ready     bool               `json:"ready"`
}

// ContainerResources represents container resource requests and limits.
type ContainerResources struct {
	Requests map[string]string `json:"requests"` // e.g., "cpu": "500m", "memory": "512Mi"
	Limits   map[string]string `json:"limits"`   // e.g., "cpu": "1", "memory": "1Gi"
}

// Node represents a Kubernetes node.
type Node struct {
	Name              string            `json:"name"`
	CreationTimestamp time.Time         `json:"creation_timestamp"`
	Labels            map[string]string `json:"labels"`
	Annotations       map[string]string `json:"annotations"`
	Conditions        []NodeCondition   `json:"conditions"`
	Capacity          map[string]string `json:"capacity"`    // e.g., "cpu": "8", "memory": "32Gi"
	Allocatable       map[string]string `json:"allocatable"` // e.g., "cpu": "7.5", "memory": "30Gi"
	Addresses         []NodeAddress     `json:"addresses"`
}

// NodeCondition represents a node condition.
type NodeCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// NodeAddress represents a node address.
type NodeAddress struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

// Event represents a Kubernetes event.
type Event struct {
	Name            string          `json:"name"`
	Namespace       string          `json:"namespace"`
	Type            string          `json:"type"`   // Normal, Warning
	Reason          string          `json:"reason"` // e.g., Scheduled, Killing, Created
	Message         string          `json:"message"`
	SourceComponent string          `json:"source_component"`
	SourceHost      string          `json:"source_host"`
	Count           int32           `json:"count"`
	FirstTimestamp  time.Time       `json:"first_timestamp"`
	LastTimestamp   time.Time       `json:"last_timestamp"`
	InvolvedObject  ObjectReference `json:"involved_object"`
}

// ObjectReference references a Kubernetes object.
type ObjectReference struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid"`
}

// ResourceQuota represents a Kubernetes resource quota.
type ResourceQuota struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	CreationTimestamp time.Time         `json:"creation_timestamp"`
	Hard              map[string]string `json:"hard"` // e.g., "cpu": "10", "memory": "20Gi"
	Used              map[string]string `json:"used"` // e.g., "cpu": "5", "memory": "8Gi"
	Scopes            []string          `json:"scopes"`
	ScopeSelector     map[string]string `json:"scope_selector"`
}
