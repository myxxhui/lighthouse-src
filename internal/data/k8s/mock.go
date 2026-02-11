// Package k8s provides mock implementations for testing.
package k8s

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"time"
)

// MockConfig defines configuration options for the mock K8s client.
type MockConfig struct {
	// Scenario defines the test scenario to simulate
	Scenario string `json:"scenario"` // "standard", "chaos", "healthy", "empty"

	// DataSize defines the size of generated data sets
	DataSize string `json:"data_size"` // "small", "medium", "large"

	// Namespaces to include in mock data
	Namespaces []string `json:"namespaces"`

	// Nodes to simulate
	Nodes []string `json:"nodes"`

	// Deployments per namespace
	DeploymentsPerNamespace int `json:"deployments_per_namespace"`

	// Pods per deployment
	PodsPerDeployment int `json:"pods_per_deployment"`

	// Events per resource
	EventsPerResource int `json:"events_per_resource"`

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
		Scenario:                "standard",
		DataSize:                "medium",
		Namespaces:              []string{"default", "kube-system", "monitoring", "app-prod", "app-staging"},
		Nodes:                   []string{"node-1", "node-2", "node-3", "node-4"},
		DeploymentsPerNamespace: 3,
		PodsPerDeployment:       2,
		EventsPerResource:       5,
		RandomSeed:              42,
		ErrorRate:               0.0,
		LatencyMs:               20,
	}
}

// MockClient is a mock implementation of the K8s Client interface.
type MockClient struct {
	config MockConfig
	rand   *rand.Rand
}

// NewMockClient creates a new mock K8s client with the given configuration.
func NewMockClient(config MockConfig) *MockClient {
	if config.RandomSeed == 0 {
		config.RandomSeed = time.Now().UnixNano()
	}
	return &MockClient{
		config: config,
		rand:   rand.New(rand.NewSource(config.RandomSeed)),
	}
}

// GetNamespaces retrieves mock namespaces.
func (m *MockClient) GetNamespaces(ctx context.Context) ([]Namespace, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock K8s error: cannot get namespaces")
	}

	if m.config.Scenario == "empty" {
		return []Namespace{}, nil
	}

	var namespaces []Namespace
	for _, nsName := range m.config.Namespaces {
		ns := Namespace{
			Name:              nsName,
			CreationTimestamp: time.Now().Add(-time.Duration(m.rand.Intn(365)) * 24 * time.Hour),
			Labels:            m.generateLabels("namespace", nsName),
			Annotations:       m.generateAnnotations("namespace", nsName),
			Status:            "Active",
		}
		namespaces = append(namespaces, ns)
	}

	return namespaces, nil
}

// GetDeployments retrieves mock deployments for a namespace.
func (m *MockClient) GetDeployments(ctx context.Context, namespace string) ([]Deployment, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock K8s error: cannot get deployments for namespace %s", namespace)
	}

	if m.config.Scenario == "empty" {
		return []Deployment{}, nil
	}

	var deployments []Deployment
	deploymentCount := m.getResourceCount("deployments")

	for i := 0; i < deploymentCount; i++ {
		deploymentName := fmt.Sprintf("%s-deployment-%d", namespace, i+1)
		replicas := int32(1 + m.rand.Intn(5))

		// Adjust based on scenario
		if m.config.Scenario == "chaos" && m.rand.Float64() > 0.7 {
			replicas = 0 // Some deployments with no replicas in chaos scenario
		}

		deployment := Deployment{
			Name:              deploymentName,
			Namespace:         namespace,
			Replicas:          replicas,
			AvailableReplicas: replicas,
			Labels:            m.generateLabels("deployment", deploymentName),
			Annotations:       m.generateAnnotations("deployment", deploymentName),
			CreationTimestamp: time.Now().Add(-time.Duration(m.rand.Intn(30)) * 24 * time.Hour),
			StrategyType:      "RollingUpdate",
		}

		// In chaos scenario, some deployments may have unavailable replicas
		if m.config.Scenario == "chaos" && m.rand.Float64() > 0.8 {
			deployment.AvailableReplicas = replicas - 1
			if deployment.AvailableReplicas < 0 {
				deployment.AvailableReplicas = 0
			}
		}

		deployments = append(deployments, deployment)
	}

	return deployments, nil
}

// GetPods retrieves mock pods for a namespace or deployment.
func (m *MockClient) GetPods(ctx context.Context, namespace, deployment string) ([]Pod, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock K8s error: cannot get pods for namespace %s", namespace)
	}

	if m.config.Scenario == "empty" {
		return []Pod{}, nil
	}

	var pods []Pod
	podCount := m.getResourceCount("pods")

	for i := 0; i < podCount; i++ {
		podName := fmt.Sprintf("%s-pod-%d", deployment, i+1)
		if deployment == "" {
			podName = fmt.Sprintf("%s-pod-%d", namespace, i+1)
		}

		// Select random node
		nodeIdx := m.rand.Intn(len(m.config.Nodes))
		nodeName := m.config.Nodes[nodeIdx]

		// Determine pod phase based on scenario
		phase := "Running"
		if m.config.Scenario == "chaos" {
			phases := []string{"Running", "Pending", "Failed", "Succeeded"}
			phase = phases[m.rand.Intn(len(phases))]
		}

		pod := Pod{
			Name:              podName,
			Namespace:         namespace,
			Deployment:        deployment,
			NodeName:          nodeName,
			Phase:             phase,
			CreationTimestamp: time.Now().Add(-time.Duration(m.rand.Intn(24)) * time.Hour),
			Labels:            m.generateLabels("pod", podName),
			Annotations:       m.generateAnnotations("pod", podName),
			Containers:        m.generateContainers(),
		}

		pods = append(pods, pod)
	}

	return pods, nil
}

// GetNodes retrieves mock cluster nodes.
func (m *MockClient) GetNodes(ctx context.Context) ([]Node, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock K8s error: cannot get nodes")
	}

	if m.config.Scenario == "empty" {
		return []Node{}, nil
	}

	var nodes []Node
	for _, nodeName := range m.config.Nodes {
		node := Node{
			Name:              nodeName,
			CreationTimestamp: time.Now().Add(-time.Duration(m.rand.Intn(180)) * 24 * time.Hour),
			Labels:            m.generateLabels("node", nodeName),
			Annotations:       m.generateAnnotations("node", nodeName),
			Conditions:        m.generateNodeConditions(),
			Capacity:          m.generateNodeResources("capacity"),
			Allocatable:       m.generateNodeResources("allocatable"),
			Addresses:         m.generateNodeAddresses(nodeName),
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// GetEvents retrieves mock events for a namespace or resource.
func (m *MockClient) GetEvents(ctx context.Context, namespace, resourceType, resourceName string) ([]Event, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock K8s error: cannot get events")
	}

	if m.config.Scenario == "empty" {
		return []Event{}, nil
	}

	var events []Event
	eventCount := m.config.EventsPerResource

	for i := 0; i < eventCount; i++ {
		eventType := "Normal"
		reason := "Scheduled"
		message := "Successfully assigned pod to node"

		if m.config.Scenario == "chaos" && m.rand.Float64() > 0.6 {
			eventType = "Warning"
			reasons := []string{"FailedScheduling", "FailedMount", "FailedPull", "CrashLoopBackOff"}
			reason = reasons[m.rand.Intn(len(reasons))]
			messages := []string{
				"0/4 nodes are available: 4 node(s) had taint {node.kubernetes.io/not-ready: }",
				"MountVolume.SetUp failed for volume",
				"Failed to pull image",
				"Back-off restarting failed container",
			}
			message = messages[m.rand.Intn(len(messages))]
		}

		event := Event{
			Name:            fmt.Sprintf("%s-event-%d", resourceName, i+1),
			Namespace:       namespace,
			Type:            eventType,
			Reason:          reason,
			Message:         message,
			SourceComponent: "kube-scheduler",
			SourceHost:      fmt.Sprintf("node-%d", m.rand.Intn(4)+1),
			Count:           int32(1 + m.rand.Intn(10)),
			FirstTimestamp:  time.Now().Add(-time.Duration(m.rand.Intn(60)) * time.Minute),
			LastTimestamp:   time.Now().Add(-time.Duration(m.rand.Intn(5)) * time.Minute),
			InvolvedObject: ObjectReference{
				Kind:      resourceType,
				Namespace: namespace,
				Name:      resourceName,
				UID:       fmt.Sprintf("uid-%s-%d", resourceName, i+1),
			},
		}
		events = append(events, event)
	}

	return events, nil
}

// GetResourceQuotas retrieves mock resource quotas for a namespace.
func (m *MockClient) GetResourceQuotas(ctx context.Context, namespace string) ([]ResourceQuota, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock K8s error: cannot get resource quotas")
	}

	if m.config.Scenario == "empty" {
		return []ResourceQuota{}, nil
	}

	var quotas []ResourceQuota
	quotaCount := 1 + m.rand.Intn(2)

	for i := 0; i < quotaCount; i++ {
		quotaName := fmt.Sprintf("%s-quota-%d", namespace, i+1)

		// Generate resource limits
		hard := map[string]string{
			"cpu":    fmt.Sprintf("%d", 10+m.rand.Intn(20)),
			"memory": fmt.Sprintf("%dGi", 20+m.rand.Intn(30)),
			"pods":   fmt.Sprintf("%d", 50+m.rand.Intn(100)),
		}

		// Generate usage (typically 30-80% of hard limits)
		used := map[string]string{
			"cpu":    fmt.Sprintf("%d", int(float64(m.parseResource(hard["cpu"]))*(0.3+m.rand.Float64()*0.5))),
			"memory": fmt.Sprintf("%dGi", int(float64(m.parseResource(hard["memory"]))*(0.3+m.rand.Float64()*0.5))),
			"pods":   fmt.Sprintf("%d", int(float64(m.parseResource(hard["pods"]))*(0.3+m.rand.Float64()*0.5))),
		}

		quota := ResourceQuota{
			Name:              quotaName,
			Namespace:         namespace,
			CreationTimestamp: time.Now().Add(-time.Duration(m.rand.Intn(30)) * 24 * time.Hour),
			Hard:              hard,
			Used:              used,
			Scopes:            []string{"NotTerminating"},
			ScopeSelector:     map[string]string{},
		}
		quotas = append(quotas, quota)
	}

	return quotas, nil
}

// HealthCheck always returns nil (healthy) for mock client.
func (m *MockClient) HealthCheck(ctx context.Context) error {
	if m.shouldReturnError() {
		return fmt.Errorf("mock K8s health check failed")
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

func (m *MockClient) getResourceCount(resourceType string) int {
	switch m.config.DataSize {
	case "small":
		switch resourceType {
		case "deployments":
			return 2
		case "pods":
			return 3
		default:
			return 5
		}
	case "large":
		switch resourceType {
		case "deployments":
			return 10
		case "pods":
			return 20
		default:
			return 30
		}
	default: // "medium"
		switch resourceType {
		case "deployments":
			return 5
		case "pods":
			return 8
		default:
			return 15
		}
	}
}

func (m *MockClient) generateLabels(resourceType, name string) map[string]string {
	labels := map[string]string{
		"app":     name,
		"version": "v" + strconv.Itoa(1+m.rand.Intn(3)),
	}

	switch resourceType {
	case "namespace":
		labels["environment"] = m.randomChoice([]string{"production", "staging", "development"})
	case "deployment":
		labels["component"] = m.randomChoice([]string{"api", "web", "worker", "database"})
	case "pod":
		labels["instance"] = strconv.Itoa(1 + m.rand.Intn(5))
	case "node":
		labels["node-type"] = m.randomChoice([]string{"compute", "storage", "gpu"})
	}

	return labels
}

func (m *MockClient) generateAnnotations(resourceType, name string) map[string]string {
	annotations := map[string]string{
		"created-by": "mock-k8s-client",
		"timestamp":  time.Now().Format(time.RFC3339),
	}

	if resourceType == "deployment" || resourceType == "pod" {
		annotations["description"] = fmt.Sprintf("Mock %s for testing", resourceType)
	}

	return annotations
}

func (m *MockClient) generateContainers() []Container {
	containerCount := 1 + m.rand.Intn(3)
	var containers []Container

	for i := 0; i < containerCount; i++ {
		containerName := fmt.Sprintf("container-%d", i+1)
		container := Container{
			Name:  containerName,
			Image: fmt.Sprintf("myapp/%s:v%d", containerName, 1+m.rand.Intn(3)),
			Resources: ContainerResources{
				Requests: map[string]string{
					"cpu":    fmt.Sprintf("%dm", 100+m.rand.Intn(900)),
					"memory": fmt.Sprintf("%dMi", 256+m.rand.Intn(768)),
				},
				Limits: map[string]string{
					"cpu":    fmt.Sprintf("%dm", 500+m.rand.Intn(1500)),
					"memory": fmt.Sprintf("%dMi", 512+m.rand.Intn(1024)),
				},
			},
			Ready: true,
		}

		// In chaos scenario, some containers may not be ready
		if m.config.Scenario == "chaos" && m.rand.Float64() > 0.8 {
			container.Ready = false
		}

		containers = append(containers, container)
	}

	return containers
}

func (m *MockClient) generateNodeConditions() []NodeCondition {
	conditions := []NodeCondition{
		{
			Type:    "Ready",
			Status:  "True",
			Reason:  "KubeletReady",
			Message: "kubelet is posting ready status",
		},
		{
			Type:    "MemoryPressure",
			Status:  "False",
			Reason:  "KubeletHasSufficientMemory",
			Message: "kubelet has sufficient memory available",
		},
		{
			Type:    "DiskPressure",
			Status:  "False",
			Reason:  "KubeletHasNoDiskPressure",
			Message: "kubelet has no disk pressure",
		},
		{
			Type:    "PIDPressure",
			Status:  "False",
			Reason:  "KubeletHasSufficientPID",
			Message: "kubelet has sufficient PID available",
		},
	}

	// In chaos scenario, some nodes may have issues
	if m.config.Scenario == "chaos" && m.rand.Float64() > 0.7 {
		conditions[0].Status = "False"
		conditions[0].Reason = "KubeletNotReady"
		conditions[0].Message = "kubelet is not posting ready status"
	}

	return conditions
}

func (m *MockClient) generateNodeResources(resourceType string) map[string]string {
	// capacity vs allocatable: allocatable is slightly less than capacity
	baseCPU := 8 + m.rand.Intn(16)
	baseMemory := 32 + m.rand.Intn(64) // GB

	if resourceType == "allocatable" {
		// Allocatable is typically 90-95% of capacity
		baseCPU = int(float64(baseCPU) * (0.9 + m.rand.Float64()*0.05))
		baseMemory = int(float64(baseMemory) * (0.9 + m.rand.Float64()*0.05))
	}

	return map[string]string{
		"cpu":               fmt.Sprintf("%d", baseCPU),
		"memory":            fmt.Sprintf("%dGi", baseMemory),
		"ephemeral-storage": fmt.Sprintf("%dGi", 100+m.rand.Intn(200)),
		"pods":              fmt.Sprintf("%d", 110+m.rand.Intn(100)),
	}
}

func (m *MockClient) generateNodeAddresses(nodeName string) []NodeAddress {
	return []NodeAddress{
		{
			Type:    "InternalIP",
			Address: fmt.Sprintf("10.0.0.%d", 10+m.rand.Intn(20)),
		},
		{
			Type:    "Hostname",
			Address: nodeName,
		},
	}
}

func (m *MockClient) parseResource(resourceStr string) int {
	// Simple parser for resource strings like "10", "20Gi"
	// In real implementation, would parse units properly
	for i := 0; i < len(resourceStr); i++ {
		if resourceStr[i] < '0' || resourceStr[i] > '9' {
			val, _ := strconv.Atoi(resourceStr[:i])
			return val
		}
	}
	val, _ := strconv.Atoi(resourceStr)
	return val
}

func (m *MockClient) randomChoice(choices []string) string {
	return choices[m.rand.Intn(len(choices))]
}
