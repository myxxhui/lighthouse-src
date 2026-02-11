// Package main provides a command-line tool for generating mock data for Lighthouse testing.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/myxxhui/lighthouse-src/internal/data/k8s"
	"github.com/myxxhui/lighthouse-src/internal/data/postgres"
	"github.com/myxxhui/lighthouse-src/internal/data/prometheus"
	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

// Scenario defines available test scenarios.
type Scenario string

const (
	ScenarioStandard   Scenario = "standard"
	ScenarioZombie     Scenario = "zombie"
	ScenarioRisk       Scenario = "risk"
	ScenarioChaos      Scenario = "chaos"
	ScenarioEmpty      Scenario = "empty"
	ScenarioHistorical Scenario = "historical"
)

// DataSize defines available data sizes.
type DataSize string

const (
	DataSizeSmall  DataSize = "small"
	DataSizeMedium DataSize = "medium"
	DataSizeLarge  DataSize = "large"
)

// Config holds the generation configuration.
type Config struct {
	Scenario    Scenario `json:"scenario"`
	DataSize    DataSize `json:"data_size"`
	OutputDir   string   `json:"output_dir"`
	Seed        int64    `json:"seed"`
	Verbose     bool     `json:"verbose"`
	GenerateAll bool     `json:"generate_all"`
}

func main() {
	var (
		scenario       = flag.String("scenario", "standard", "Test scenario: standard, zombie, risk, chaos, empty, historical")
		dataSize       = flag.String("data-size", "medium", "Data size: small, medium, large")
		outputDir      = flag.String("output-dir", "./testdata/generated", "Output directory for generated data")
		seed           = flag.Int64("seed", 42, "Random seed for deterministic generation")
		verbose        = flag.Bool("verbose", false, "Enable verbose logging")
		generateAll    = flag.Bool("all", false, "Generate all data types")
		prometheusFlag = flag.Bool("prometheus", false, "Generate Prometheus mock data")
		k8sFlag        = flag.Bool("k8s", false, "Generate K8s mock data")
		postgresFlag   = flag.Bool("postgres", false, "Generate PostgreSQL mock data")
		configFile     = flag.String("config", "", "JSON configuration file")
	)

	flag.Parse()

	// Load configuration from file if provided
	config := Config{
		Scenario:    Scenario(*scenario),
		DataSize:    DataSize(*dataSize),
		OutputDir:   *outputDir,
		Seed:        *seed,
		Verbose:     *verbose,
		GenerateAll: *generateAll,
	}

	if *configFile != "" {
		if err := loadConfig(*configFile, &config); err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
	}

	// Determine what to generate
	generatePrometheus := *prometheusFlag || config.GenerateAll
	generateK8s := *k8sFlag || config.GenerateAll
	generatePostgres := *postgresFlag || config.GenerateAll

	// If no specific flags and not generateAll, generate all by default
	if !generatePrometheus && !generateK8s && !generatePostgres && !config.GenerateAll {
		generatePrometheus = true
		generateK8s = true
		generatePostgres = true
	}

	// Create output directory
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	log.Printf("Starting mock data generation with scenario=%s, size=%s, seed=%d", config.Scenario, config.DataSize, config.Seed)

	ctx := context.Background()

	// Generate requested data types
	if generatePrometheus {
		if err := generatePrometheusData(ctx, config); err != nil {
			log.Printf("Warning: Failed to generate Prometheus data: %v", err)
		} else {
			log.Println("✓ Generated Prometheus mock data")
		}
	}

	if generateK8s {
		if err := generateK8sData(ctx, config); err != nil {
			log.Printf("Warning: Failed to generate K8s data: %v", err)
		} else {
			log.Println("✓ Generated K8s mock data")
		}
	}

	if generatePostgres {
		if err := generatePostgresData(ctx, config); err != nil {
			log.Printf("Warning: Failed to generate PostgreSQL data: %v", err)
		} else {
			log.Println("✓ Generated PostgreSQL mock data")
		}
	}

	log.Println("✅ Mock data generation completed successfully!")
}

func loadConfig(filename string, config *Config) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, config)
}

func generatePrometheusData(ctx context.Context, config Config) error {
	// Create Prometheus mock client
	promConfig := prometheus.MockConfig{
		Scenario:              string(config.Scenario),
		DataSize:              string(config.DataSize),
		Namespaces:            []string{"default", "kube-system", "monitoring", "app-prod", "app-staging"},
		WorkloadsPerNamespace: 3,
		PodsPerWorkload:       2,
		Nodes:                 []string{"node-1", "node-2", "node-3", "node-4"},
		RandomSeed:            config.Seed,
		ErrorRate:             0.0,
		LatencyMs:             0,
	}

	client := prometheus.NewMockClient(promConfig)

	// Generate sample data
	startTime := time.Now().Add(-24 * time.Hour)
	endTime := time.Now()

	// Get resource metrics
	metrics, err := client.GetResourceMetrics(ctx, "default", "sample-deployment", "sample-pod", startTime, endTime)
	if err != nil {
		return fmt.Errorf("failed to get resource metrics: %w", err)
	}

	// Get node metrics
	nodeMetrics, err := client.GetNodeMetrics(ctx, "node-1", startTime, endTime)
	if err != nil {
		return fmt.Errorf("failed to get node metrics: %w", err)
	}

	// Get throttling metrics
	throttlingMetrics, err := client.GetThrottlingMetrics(ctx, "default", "sample-pod", startTime, endTime)
	if err != nil {
		return fmt.Errorf("failed to get throttling metrics: %w", err)
	}

	// Save generated data
	data := map[string]interface{}{
		"config":             promConfig,
		"resource_metrics":   metrics,
		"node_metrics":       nodeMetrics,
		"throttling_metrics": throttlingMetrics,
		"generated_at":       time.Now(),
	}

	return saveJSON(config.OutputDir+"/prometheus_data.json", data)
}

func generateK8sData(ctx context.Context, config Config) error {
	// Create K8s mock client
	k8sConfig := k8s.MockConfig{
		Scenario:                string(config.Scenario),
		DataSize:                string(config.DataSize),
		Namespaces:              []string{"default", "kube-system", "monitoring", "app-prod", "app-staging"},
		Nodes:                   []string{"node-1", "node-2", "node-3", "node-4"},
		DeploymentsPerNamespace: 3,
		PodsPerDeployment:       2,
		EventsPerResource:       5,
		RandomSeed:              config.Seed,
		ErrorRate:               0.0,
		LatencyMs:               0,
	}

	client := k8s.NewMockClient(k8sConfig)

	// Generate sample data
	namespaces, err := client.GetNamespaces(ctx)
	if err != nil {
		return fmt.Errorf("failed to get namespaces: %w", err)
	}

	deployments, err := client.GetDeployments(ctx, "default")
	if err != nil {
		return fmt.Errorf("failed to get deployments: %w", err)
	}

	pods, err := client.GetPods(ctx, "default", "")
	if err != nil {
		return fmt.Errorf("failed to get pods: %w", err)
	}

	nodes, err := client.GetNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	events, err := client.GetEvents(ctx, "default", "Pod", "sample-pod")
	if err != nil {
		return fmt.Errorf("failed to get events: %w", err)
	}

	// Save generated data
	data := map[string]interface{}{
		"config":       k8sConfig,
		"namespaces":   namespaces,
		"deployments":  deployments,
		"pods":         pods,
		"nodes":        nodes,
		"events":       events,
		"generated_at": time.Now(),
	}

	return saveJSON(config.OutputDir+"/k8s_data.json", data)
}

func generatePostgresData(ctx context.Context, config Config) error {
	// Create PostgreSQL mock repository
	postgresConfig := postgres.MockConfig{
		Scenario: string(config.Scenario),
		DataSize: string(config.DataSize),
		InitialDataCount: map[string]int{
			"cost_snapshots":        getDataCount(config.DataSize, 5, 20, 50),
			"roi_baselines":         getDataCount(config.DataSize, 2, 5, 10),
			"daily_namespace_costs": getDataCount(config.DataSize, 10, 30, 100),
			"hourly_workload_stats": getDataCount(config.DataSize, 50, 100, 500),
			"metadata":              getDataCount(config.DataSize, 5, 10, 20),
		},
		Namespaces:            []string{"default", "kube-system", "monitoring", "app-prod", "app-staging"},
		WorkloadsPerNamespace: 3,
		RandomSeed:            config.Seed,
		ErrorRate:             0.0,
		LatencyMs:             0,
		EnableTransactions:    true,
	}

	repo := postgres.NewMockRepository(postgresConfig)

	// Generate sample queries
	costSnapshots, err := repo.ListCostSnapshots(ctx, postgres.CostSnapshotFilter{
		Limit: 10,
	})
	if err != nil {
		return fmt.Errorf("failed to list cost snapshots: %w", err)
	}

	roiBaselines, err := repo.ListROIBaselines(ctx, postgres.ROIBaselineFilter{
		Limit: 5,
	})
	if err != nil {
		return fmt.Errorf("failed to list ROI baselines: %w", err)
	}

	dailyCosts, err := repo.ListDailyNamespaceCosts(ctx, postgres.DailyNamespaceCostFilter{
		Namespace: "default",
		Limit:     7,
	})
	if err != nil {
		return fmt.Errorf("failed to list daily namespace costs: %w", err)
	}

	workloadStats, err := repo.ListHourlyWorkloadStats(ctx, postgres.HourlyWorkloadStatFilter{
		Namespace: "default",
		Limit:     24,
	})
	if err != nil {
		return fmt.Errorf("failed to list hourly workload stats: %w", err)
	}

	// Create a sample cost snapshot
	sampleSnapshot := postgres.CostSnapshot{
		ID:                     "sample-snapshot-001",
		CalculationID:          "calc-001",
		Timestamp:              time.Now(),
		TimeRangeStart:         time.Now().Add(-24 * time.Hour),
		TimeRangeEnd:           time.Now(),
		ResourceResults:        generateSampleCostResults(),
		AggregatedResults:      make(map[costmodel.AggregationLevel][]costmodel.AggregationResult),
		TotalBillableCost:      1250.75,
		TotalUsageCost:         675.50,
		TotalWasteCost:         125.25,
		OverallEfficiencyScore: 0.78,
		ZombieCount:            2,
		OverProvisionedCount:   5,
		HealthyCount:           15,
		RiskCount:              1,
		Metadata:               map[string]interface{}{"scenario": config.Scenario, "generated_by": "mock-tool"},
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
	}

	if err := repo.SaveCostSnapshot(ctx, sampleSnapshot); err != nil {
		return fmt.Errorf("failed to save sample cost snapshot: %w", err)
	}

	// Save generated data
	data := map[string]interface{}{
		"config":                postgresConfig,
		"cost_snapshots":        costSnapshots,
		"roi_baselines":         roiBaselines,
		"daily_namespace_costs": dailyCosts,
		"hourly_workload_stats": workloadStats,
		"sample_snapshot":       sampleSnapshot,
		"repository_stats": map[string]int{
			"cost_snapshots":        len(costSnapshots),
			"roi_baselines":         len(roiBaselines),
			"daily_namespace_costs": len(dailyCosts),
			"hourly_workload_stats": len(workloadStats),
		},
		"generated_at": time.Now(),
	}

	return saveJSON(config.OutputDir+"/postgres_data.json", data)
}

func generateSampleCostResults() []costmodel.CostResult {
	return []costmodel.CostResult{
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
		{
			CPUBillableCost:        100.0,
			CPUUsageCost:           90.0,
			CPUWasteCost:           5.0,
			CPUEfficiencyScore:     0.95,
			MemBillableCost:        150.0,
			MemUsageCost:           145.0,
			MemWasteCost:           2.0,
			MemEfficiencyScore:     0.98,
			TotalBillableCost:      250.0,
			TotalUsageCost:         235.0,
			TotalWasteCost:         7.0,
			OverallEfficiencyScore: 0.96,
			OverallGrade:           costmodel.GradeRisk,
		},
	}
}

func getDataCount(size DataSize, small, medium, large int) int {
	switch size {
	case DataSizeSmall:
		return small
	case DataSizeLarge:
		return large
	default: // DataSizeMedium
		return medium
	}
}

func saveJSON(filename string, data interface{}) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}
