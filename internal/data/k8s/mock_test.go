package k8s

import (
	"context"
	"testing"
	"time"
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

func TestMockClient_GetNamespaces(t *testing.T) {
	ctx := context.Background()
	client := NewMockClient(DefaultMockConfig())

	namespaces, err := client.GetNamespaces(ctx)
	if err != nil {
		t.Fatalf("GetNamespaces failed: %v", err)
	}

	if len(namespaces) == 0 {
		t.Error("Expected non-empty namespaces")
	}

	for _, ns := range namespaces {
		if ns.Name == "" {
			t.Error("Namespace name should not be empty")
		}
		if ns.Status != "Active" {
			t.Errorf("Expected status Active, got %s", ns.Status)
		}
		if ns.CreationTimestamp.IsZero() {
			t.Error("Expected non-zero creation timestamp")
		}
	}
}

func TestMockClient_GetDeployments(t *testing.T) {
	ctx := context.Background()
	client := NewMockClient(DefaultMockConfig())

	deployments, err := client.GetDeployments(ctx, "default")
	if err != nil {
		t.Fatalf("GetDeployments failed: %v", err)
	}

	if len(deployments) == 0 {
		t.Error("Expected non-empty deployments")
	}

	for _, deployment := range deployments {
		if deployment.Name == "" {
			t.Error("Deployment name should not be empty")
		}
		if deployment.Namespace != "default" {
			t.Errorf("Expected namespace default, got %s", deployment.Namespace)
		}
		if deployment.Replicas < 0 {
			t.Errorf("Expected non-negative replicas, got %d", deployment.Replicas)
		}
		if deployment.AvailableReplicas < 0 || deployment.AvailableReplicas > deployment.Replicas {
			t.Errorf("Available replicas %d should be between 0 and total replicas %d",
				deployment.AvailableReplicas, deployment.Replicas)
		}
		if deployment.CreationTimestamp.IsZero() {
			t.Error("Expected non-zero creation timestamp")
		}
	}
}

func TestMockClient_GetPods(t *testing.T) {
	ctx := context.Background()
	client := NewMockClient(DefaultMockConfig())

	pods, err := client.GetPods(ctx, "default", "")
	if err != nil {
		t.Fatalf("GetPods failed: %v", err)
	}

	if len(pods) == 0 {
		t.Error("Expected non-empty pods")
	}

	validPhases := map[string]bool{
		"Running": true, "Pending": true, "Failed": true, "Succeeded": true,
	}

	for _, pod := range pods {
		if pod.Name == "" {
			t.Error("Pod name should not be empty")
		}
		if pod.Namespace != "default" {
			t.Errorf("Expected namespace default, got %s", pod.Namespace)
		}
		if !validPhases[pod.Phase] {
			t.Errorf("Invalid pod phase: %s", pod.Phase)
		}
		if pod.NodeName == "" {
			t.Error("Pod should have a node name")
		}
		if pod.CreationTimestamp.IsZero() {
			t.Error("Expected non-zero creation timestamp")
		}
		if len(pod.Containers) == 0 {
			t.Error("Pod should have at least one container")
		}

		for _, container := range pod.Containers {
			if container.Name == "" {
				t.Error("Container name should not be empty")
			}
			if container.Image == "" {
				t.Error("Container image should not be empty")
			}
		}
	}
}

func TestMockClient_GetNodes(t *testing.T) {
	ctx := context.Background()
	client := NewMockClient(DefaultMockConfig())

	nodes, err := client.GetNodes(ctx)
	if err != nil {
		t.Fatalf("GetNodes failed: %v", err)
	}

	if len(nodes) == 0 {
		t.Error("Expected non-empty nodes")
	}

	for _, node := range nodes {
		if node.Name == "" {
			t.Error("Node name should not be empty")
		}
		if node.CreationTimestamp.IsZero() {
			t.Error("Expected non-zero creation timestamp")
		}
		if len(node.Conditions) == 0 {
			t.Error("Node should have conditions")
		}
		if len(node.Capacity) == 0 {
			t.Error("Node should have capacity")
		}
		if len(node.Allocatable) == 0 {
			t.Error("Node should have allocatable resources")
		}
		if len(node.Addresses) == 0 {
			t.Error("Node should have addresses")
		}

		// Check node conditions
		foundReady := false
		for _, condition := range node.Conditions {
			if condition.Type == "Ready" {
				foundReady = true
				if condition.Status != "True" && condition.Status != "False" {
					t.Errorf("Invalid Ready condition status: %s", condition.Status)
				}
			}
		}
		if !foundReady {
			t.Error("Node should have Ready condition")
		}

		// Check resources
		if _, hasCPU := node.Capacity["cpu"]; !hasCPU {
			t.Error("Node capacity should have CPU")
		}
		if _, hasMemory := node.Capacity["memory"]; !hasMemory {
			t.Error("Node capacity should have memory")
		}
	}
}

func TestMockClient_GetEvents(t *testing.T) {
	ctx := context.Background()
	client := NewMockClient(DefaultMockConfig())

	events, err := client.GetEvents(ctx, "default", "Pod", "test-pod")
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}

	if len(events) == 0 {
		t.Error("Expected non-empty events")
	}

	validTypes := map[string]bool{"Normal": true, "Warning": true}

	for _, event := range events {
		if event.Name == "" {
			t.Error("Event name should not be empty")
		}
		if event.Namespace != "default" {
			t.Errorf("Expected namespace default, got %s", event.Namespace)
		}
		if !validTypes[event.Type] {
			t.Errorf("Invalid event type: %s", event.Type)
		}
		if event.Reason == "" {
			t.Error("Event reason should not be empty")
		}
		if event.Message == "" {
			t.Error("Event message should not be empty")
		}
		if event.Count <= 0 {
			t.Errorf("Event count should be > 0, got %d", event.Count)
		}
		if event.FirstTimestamp.IsZero() {
			t.Error("Expected non-zero first timestamp")
		}
		if event.LastTimestamp.IsZero() {
			t.Error("Expected non-zero last timestamp")
		}
		if event.LastTimestamp.Before(event.FirstTimestamp) {
			t.Error("Last timestamp should not be before first timestamp")
		}

		// Check involved object
		if event.InvolvedObject.Kind != "Pod" {
			t.Errorf("Expected involved object kind Pod, got %s", event.InvolvedObject.Kind)
		}
		if event.InvolvedObject.Namespace != "default" {
			t.Errorf("Expected involved object namespace default, got %s", event.InvolvedObject.Namespace)
		}
		if event.InvolvedObject.Name != "test-pod" {
			t.Errorf("Expected involved object name test-pod, got %s", event.InvolvedObject.Name)
		}
	}
}

func TestMockClient_GetResourceQuotas(t *testing.T) {
	ctx := context.Background()
	client := NewMockClient(DefaultMockConfig())

	quotas, err := client.GetResourceQuotas(ctx, "default")
	if err != nil {
		t.Fatalf("GetResourceQuotas failed: %v", err)
	}

	// There might be zero or more quotas
	for _, quota := range quotas {
		if quota.Name == "" {
			t.Error("Quota name should not be empty")
		}
		if quota.Namespace != "default" {
			t.Errorf("Expected namespace default, got %s", quota.Namespace)
		}
		if quota.CreationTimestamp.IsZero() {
			t.Error("Expected non-zero creation timestamp")
		}
		if len(quota.Hard) == 0 {
			t.Error("Quota should have hard limits")
		}
		if len(quota.Used) == 0 {
			t.Error("Quota should have usage")
		}

		// Check that used <= hard (simplified check)
		for resource, hardStr := range quota.Hard {
			usedStr, hasUsed := quota.Used[resource]
			if hasUsed {
				// In a real test, we would parse the resource strings
				// For now, just check they're not empty
				if hardStr == "" {
					t.Errorf("Hard limit for %s should not be empty", resource)
				}
				if usedStr == "" {
					t.Errorf("Used amount for %s should not be empty", resource)
				}
			}
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
		check    func([]Namespace, []Deployment, []Pod)
	}{
		{
			name:     "Standard scenario",
			scenario: "standard",
			check: func(namespaces []Namespace, deployments []Deployment, pods []Pod) {
				if len(namespaces) == 0 {
					t.Error("Expected non-empty namespaces in standard scenario")
				}
				if len(deployments) == 0 {
					t.Error("Expected non-empty deployments in standard scenario")
				}
				if len(pods) == 0 {
					t.Error("Expected non-empty pods in standard scenario")
				}

				// All pods should be running in standard scenario
				for _, pod := range pods {
					if pod.Phase != "Running" {
						t.Errorf("Expected all pods Running in standard scenario, got %s", pod.Phase)
					}
				}
			},
		},
		{
			name:     "Chaos scenario",
			scenario: "chaos",
			check: func(namespaces []Namespace, deployments []Deployment, pods []Pod) {
				// In chaos scenario, we should see some non-running pods
				nonRunningCount := 0
				for _, pod := range pods {
					if pod.Phase != "Running" {
						nonRunningCount++
					}
				}
				if nonRunningCount == 0 {
					t.Error("Chaos scenario should have some non-running pods")
				}

				// Some deployments might have unavailable replicas
				unavailableCount := 0
				for _, deployment := range deployments {
					if deployment.AvailableReplicas < deployment.Replicas {
						unavailableCount++
					}
				}
				if unavailableCount == 0 {
					t.Error("Chaos scenario should have some deployments with unavailable replicas")
				}
			},
		},
		{
			name:     "Empty scenario",
			scenario: "empty",
			check: func(namespaces []Namespace, deployments []Deployment, pods []Pod) {
				if len(namespaces) != 0 {
					t.Errorf("Empty scenario: expected empty namespaces, got %d", len(namespaces))
				}
				if len(deployments) != 0 {
					t.Errorf("Empty scenario: expected empty deployments, got %d", len(deployments))
				}
				if len(pods) != 0 {
					t.Errorf("Empty scenario: expected empty pods, got %d", len(pods))
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

			namespaces, err1 := client.GetNamespaces(ctx)
			if err1 != nil {
				t.Fatalf("GetNamespaces failed: %v", err1)
			}

			deployments, err2 := client.GetDeployments(ctx, "default")
			if err2 != nil {
				t.Fatalf("GetDeployments failed: %v", err2)
			}

			pods, err3 := client.GetPods(ctx, "default", "")
			if err3 != nil {
				t.Fatalf("GetPods failed: %v", err3)
			}

			tc.check(namespaces, deployments, pods)
		})
	}
}

func TestMockClient_DataSizeVariations(t *testing.T) {
	testCases := []struct {
		name     string
		dataSize string
		minPods  int
		maxPods  int
	}{
		{"Small data size", "small", 2, 4},
		{"Medium data size", "medium", 7, 9},
		{"Large data size", "large", 18, 22},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			config := DefaultMockConfig()
			config.DataSize = tc.dataSize
			client := NewMockClient(config)

			pods, err := client.GetPods(ctx, "default", "")
			if err != nil {
				t.Fatalf("GetPods failed: %v", err)
			}

			if len(pods) < tc.minPods {
				t.Errorf("Expected at least %d pods, got %d", tc.minPods, len(pods))
			}
			if len(pods) > tc.maxPods {
				t.Errorf("Expected at most %d pods, got %d", tc.maxPods, len(pods))
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

	namespaces1, err1 := client1.GetNamespaces(ctx)
	if err1 != nil {
		t.Fatalf("Client1 GetNamespaces failed: %v", err1)
	}

	namespaces2, err2 := client2.GetNamespaces(ctx)
	if err2 != nil {
		t.Fatalf("Client2 GetNamespaces failed: %v", err2)
	}

	// Should generate identical data with same seed
	if len(namespaces1) != len(namespaces2) {
		t.Errorf("Namespace count mismatch: %d != %d", len(namespaces1), len(namespaces2))
	}

	for i := range namespaces1 {
		if namespaces1[i].Name != namespaces2[i].Name {
			t.Errorf("Namespace name mismatch at index %d: %s != %s",
				i, namespaces1[i].Name, namespaces2[i].Name)
		}
	}
}

func TestMockClient_WithLatency(t *testing.T) {
	ctx := context.Background()
	config := DefaultMockConfig()
	config.LatencyMs = 20 // 20ms latency
	client := NewMockClient(config)

	start := time.Now()
	_, err := client.GetNamespaces(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("GetNamespaces failed: %v", err)
	}

	// Should take at least the configured latency
	if elapsed < 20*time.Millisecond {
		t.Errorf("Expected at least 20ms latency, got %v", elapsed)
	}
}
