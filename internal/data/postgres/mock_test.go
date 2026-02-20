package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

func TestNewMockRepository(t *testing.T) {
	config := DefaultMockConfig()
	repo := NewMockRepository(config)

	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
}

func TestMockRepository_SaveAndGetCostSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository(DefaultMockConfig())

	snapshot := CostSnapshot{
		ID:                     "test-snapshot-001",
		CalculationID:          "test-calculation-001",
		Timestamp:              time.Now(),
		TimeRangeStart:         time.Now().Add(-24 * time.Hour),
		TimeRangeEnd:           time.Now(),
		ResourceResults:        []costmodel.CostResult{},
		AggregatedResults:      make(map[costmodel.AggregationLevel][]costmodel.AggregationResult),
		TotalBillableCost:      1000.0,
		TotalUsageCost:         500.0,
		TotalWasteCost:         100.0,
		OverallEfficiencyScore: 0.8,
		ZombieCount:            2,
		OverProvisionedCount:   3,
		HealthyCount:           10,
		RiskCount:              1,
		Metadata:               map[string]interface{}{"test": true},
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
	}

	// Save snapshot
	if err := repo.SaveCostSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("SaveCostSnapshot failed: %v", err)
	}

	// Get snapshot
	retrieved, err := repo.GetCostSnapshot(ctx, "test-snapshot-001")
	if err != nil {
		t.Fatalf("GetCostSnapshot failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil snapshot")
	}
	if retrieved.ID != snapshot.ID {
		t.Errorf("Expected ID %s, got %s", snapshot.ID, retrieved.ID)
	}
	if retrieved.CalculationID != snapshot.CalculationID {
		t.Errorf("Expected CalculationID %s, got %s", snapshot.CalculationID, retrieved.CalculationID)
	}
	if retrieved.TotalBillableCost != snapshot.TotalBillableCost {
		t.Errorf("Expected TotalBillableCost %f, got %f", snapshot.TotalBillableCost, retrieved.TotalBillableCost)
	}
}

func TestMockRepository_ListCostSnapshots(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository(DefaultMockConfig())

	// List all snapshots
	snapshots, err := repo.ListCostSnapshots(ctx, CostSnapshotFilter{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListCostSnapshots failed: %v", err)
	}

	if len(snapshots) == 0 {
		t.Error("Expected non-empty snapshots (repository should be pre-populated)")
	}

	// Test filtering
	filtered, err := repo.ListCostSnapshots(ctx, CostSnapshotFilter{
		MinTotalCost: 500.0,
		Limit:        5,
	})
	if err != nil {
		t.Fatalf("ListCostSnapshots with filter failed: %v", err)
	}

	// All filtered snapshots should have total cost >= 500
	for _, snapshot := range filtered {
		if snapshot.TotalBillableCost < 500.0 {
			t.Errorf("Filtered snapshot has total cost %f < 500", snapshot.TotalBillableCost)
		}
	}
}

func TestMockRepository_DeleteCostSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository(DefaultMockConfig())

	// First, save a snapshot
	snapshot := CostSnapshot{
		ID:                "delete-test-snapshot",
		CalculationID:     "delete-test-calculation",
		Timestamp:         time.Now(),
		TotalBillableCost: 100.0,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := repo.SaveCostSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("SaveCostSnapshot failed: %v", err)
	}

	// Verify it exists
	_, err := repo.GetCostSnapshot(ctx, "delete-test-snapshot")
	if err != nil {
		t.Fatalf("GetCostSnapshot failed before delete: %v", err)
	}

	// Delete it
	if err := repo.DeleteCostSnapshot(ctx, "delete-test-snapshot"); err != nil {
		t.Fatalf("DeleteCostSnapshot failed: %v", err)
	}

	// Verify it's gone
	_, err = repo.GetCostSnapshot(ctx, "delete-test-snapshot")
	if err == nil {
		t.Error("Expected error after deleting snapshot, got nil")
	}
}

func TestMockRepository_SaveAndGetROIBaseline(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository(DefaultMockConfig())

	baseline := ROIBaseline{
		ID:              "test-baseline-001",
		Name:            "Test Baseline",
		Description:     "A test ROI baseline",
		BaselineType:    "historical",
		TimePeriodStart: time.Now().Add(-30 * 24 * time.Hour),
		TimePeriodEnd:   time.Now(),
		Metrics:         map[string]float64{"efficiency_score": 0.85, "waste_percentage": 0.15},
		ReferenceData:   map[string]interface{}{"source": "test"},
		CreatedBy:       "test-user",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Save baseline
	if err := repo.SaveROIBaseline(ctx, baseline); err != nil {
		t.Fatalf("SaveROIBaseline failed: %v", err)
	}

	// Get baseline
	retrieved, err := repo.GetROIBaseline(ctx, "test-baseline-001")
	if err != nil {
		t.Fatalf("GetROIBaseline failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil baseline")
	}
	if retrieved.Name != baseline.Name {
		t.Errorf("Expected Name %s, got %s", baseline.Name, retrieved.Name)
	}
	if retrieved.BaselineType != baseline.BaselineType {
		t.Errorf("Expected BaselineType %s, got %s", baseline.BaselineType, retrieved.BaselineType)
	}
	if retrieved.Metrics["efficiency_score"] != baseline.Metrics["efficiency_score"] {
		t.Errorf("Expected efficiency_score %f, got %f",
			baseline.Metrics["efficiency_score"], retrieved.Metrics["efficiency_score"])
	}
}

func TestMockRepository_ListROIBaselines(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository(DefaultMockConfig())

	baselines, err := repo.ListROIBaselines(ctx, ROIBaselineFilter{
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("ListROIBaselines failed: %v", err)
	}

	if len(baselines) == 0 {
		t.Error("Expected non-empty baselines (repository should be pre-populated)")
	}

	// Test filtering by type
	historicalBaselines, err := repo.ListROIBaselines(ctx, ROIBaselineFilter{
		BaselineType: "historical",
		Limit:        3,
	})
	if err != nil {
		t.Fatalf("ListROIBaselines with filter failed: %v", err)
	}

	for _, baseline := range historicalBaselines {
		if baseline.BaselineType != "historical" {
			t.Errorf("Filtered baseline has type %s, expected historical", baseline.BaselineType)
		}
	}
}

func TestMockRepository_DailyNamespaceCostOperations(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository(DefaultMockConfig())

	cost := DailyNamespaceCost{
		Namespace:       "test-namespace",
		Date:            time.Now().Truncate(24 * time.Hour),
		BillableCost:    2000.0,
		UsageCost:       1200.0,
		WasteCost:       200.0,
		PodCount:        15,
		NodeCount:       3,
		WorkloadCount:   8,
		EfficiencyScore: 0.75,
		CreatedAt:       time.Now(),
	}

	// Save cost
	if err := repo.SaveDailyNamespaceCost(ctx, cost); err != nil {
		t.Fatalf("SaveDailyNamespaceCost failed: %v", err)
	}

	// Get cost
	retrieved, err := repo.GetDailyNamespaceCost(ctx, "test-namespace", cost.Date)
	if err != nil {
		t.Fatalf("GetDailyNamespaceCost failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil cost")
	}
	if retrieved.Namespace != cost.Namespace {
		t.Errorf("Expected Namespace %s, got %s", cost.Namespace, retrieved.Namespace)
	}
	if retrieved.BillableCost != cost.BillableCost {
		t.Errorf("Expected BillableCost %f, got %f", cost.BillableCost, retrieved.BillableCost)
	}
	if retrieved.EfficiencyScore != cost.EfficiencyScore {
		t.Errorf("Expected EfficiencyScore %f, got %f", cost.EfficiencyScore, retrieved.EfficiencyScore)
	}

	// List costs
	costs, err := repo.ListDailyNamespaceCosts(ctx, DailyNamespaceCostFilter{
		Namespace: "test-namespace",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("ListDailyNamespaceCosts failed: %v", err)
	}

	found := false
	for _, c := range costs {
		if c.Namespace == "test-namespace" && c.Date.Equal(cost.Date) {
			found = true
			break
		}
	}
	if !found {
		t.Error("Saved cost not found in list")
	}

	// Aggregate costs
	startDate := time.Now().Add(-7 * 24 * time.Hour)
	endDate := time.Now()
	aggregated, err := repo.AggregateDailyNamespaceCosts(ctx, startDate, endDate)
	if err != nil {
		t.Fatalf("AggregateDailyNamespaceCosts failed: %v", err)
	}

	// Should have at least one aggregated result
	if len(aggregated) == 0 {
		t.Error("Expected non-empty aggregated results")
	}
}

func TestMockRepository_HourlyWorkloadStatOperations(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository(DefaultMockConfig())

	stat := HourlyWorkloadStat{
		Namespace:         "test-namespace",
		WorkloadName:      "test-workload",
		WorkloadType:      "Deployment",
		NodeName:          "node-1",
		PodName:           "test-pod-1",
		Timestamp:         time.Now().Truncate(time.Hour),
		CPURequest:        2.5,
		CPUUsageP95:       1.2,
		MemRequest:        4 * 1024 * 1024 * 1024, // 4GB
		MemUsageP95:       2 * 1024 * 1024 * 1024, // 2GB
		CPUBillableCost:   50.0,
		CPUUsageCost:      24.0,
		CPUWasteCost:      6.0,
		MemBillableCost:   80.0,
		MemUsageCost:      40.0,
		MemWasteCost:      10,
		TotalBillableCost: 130.0,
		TotalUsageCost:    64.0,
		TotalWasteCost:    16.0,
	}

	// Save stat
	if err := repo.SaveHourlyWorkloadStat(ctx, stat); err != nil {
		t.Fatalf("SaveHourlyWorkloadStat failed: %v", err)
	}

	// Get stat
	retrieved, err := repo.GetHourlyWorkloadStat(ctx, "test-namespace", "test-workload", stat.Timestamp)
	if err != nil {
		t.Fatalf("GetHourlyWorkloadStat failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil stat")
	}
	if retrieved.WorkloadName != stat.WorkloadName {
		t.Errorf("Expected WorkloadName %s, got %s", stat.WorkloadName, retrieved.WorkloadName)
	}
	if retrieved.CPURequest != stat.CPURequest {
		t.Errorf("Expected CPURequest %f, got %f", stat.CPURequest, retrieved.CPURequest)
	}
	if retrieved.TotalBillableCost != stat.TotalBillableCost {
		t.Errorf("Expected TotalBillableCost %f, got %f", stat.TotalBillableCost, retrieved.TotalBillableCost)
	}

	// List stats
	stats, err := repo.ListHourlyWorkloadStats(ctx, HourlyWorkloadStatFilter{
		Namespace: "test-namespace",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("ListHourlyWorkloadStats failed: %v", err)
	}

	found := false
	for _, s := range stats {
		if s.Namespace == "test-namespace" && s.WorkloadName == "test-workload" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Saved stat not found in list")
	}

	// Aggregate stats
	startTime := time.Now().Add(-24 * time.Hour)
	endTime := time.Now()
	aggregated, err := repo.AggregateHourlyWorkloadStats(ctx, startTime, endTime)
	if err != nil {
		t.Fatalf("AggregateHourlyWorkloadStats failed: %v", err)
	}

	// Should have at least one aggregated result
	if len(aggregated) == 0 {
		t.Error("Expected non-empty aggregated stats")
	}
}

// TestMockRepository_BillAccountSummary 验证总账单表 cost_bill_account_summary Mock 读写（Phase3 必做）。
func TestMockRepository_BillAccountSummary(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository(DefaultMockConfig())

	periodStart := time.Now().Truncate(24 * time.Hour)
	periodEnd := periodStart.Add(24 * time.Hour)
	summary := BillAccountSummary{
		AccountID:   "test-account",
		PeriodType:  "day",
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		TotalAmount: 10000.50,
		Currency:    "CNY",
		ByCategory:  map[string]float64{"compute": 6000, "storage": 2000, "network": 1000, "other": 1000.50},
	}

	if err := repo.SaveBillAccountSummary(ctx, summary); err != nil {
		t.Fatalf("SaveBillAccountSummary failed: %v", err)
	}

	got, err := repo.GetBillAccountSummary(ctx, summary.AccountID, summary.PeriodType, summary.PeriodStart)
	if err != nil {
		t.Fatalf("GetBillAccountSummary failed: %v", err)
	}
	if got.TotalAmount != summary.TotalAmount || got.Currency != summary.Currency {
		t.Errorf("GetBillAccountSummary: got TotalAmount=%v Currency=%s, want %v %s", got.TotalAmount, got.Currency, summary.TotalAmount, summary.Currency)
	}

	list, err := repo.ListBillAccountSummaries(ctx, summary.AccountID)
	if err != nil {
		t.Fatalf("ListBillAccountSummaries failed: %v", err)
	}
	if len(list) < 1 {
		t.Error("ListBillAccountSummaries: expected at least one record")
	}
}

func TestMockRepository_MetadataOperations(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository(DefaultMockConfig())

	metadata := Metadata{
		Key:         "test.key",
		Value:       map[string]interface{}{"setting": "value", "enabled": true},
		Description: "Test metadata",
		CreatedBy:   "test-user",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Save metadata
	if err := repo.SaveMetadata(ctx, metadata); err != nil {
		t.Fatalf("SaveMetadata failed: %v", err)
	}

	// Get metadata
	retrieved, err := repo.GetMetadata(ctx, "test.key")
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil metadata")
	}
	if retrieved.Key != metadata.Key {
		t.Errorf("Expected Key %s, got %s", metadata.Key, retrieved.Key)
	}
	if retrieved.Description != metadata.Description {
		t.Errorf("Expected Description %s, got %s", metadata.Description, retrieved.Description)
	}
	if val, ok := retrieved.Value["enabled"].(bool); !ok || !val {
		t.Error("Expected enabled=true in metadata value")
	}

	// List metadata
	metadataList, err := repo.ListMetadata(ctx, MetadataFilter{
		KeyPrefix: "test",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("ListMetadata failed: %v", err)
	}

	found := false
	for _, m := range metadataList {
		if m.Key == "test.key" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Saved metadata not found in list")
	}

	// Delete metadata
	if err := repo.DeleteMetadata(ctx, "test.key"); err != nil {
		t.Fatalf("DeleteMetadata failed: %v", err)
	}

	// Verify it's gone
	_, err = repo.GetMetadata(ctx, "test.key")
	if err == nil {
		t.Error("Expected error after deleting metadata, got nil")
	}
}

func TestMockRepository_HealthCheck(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository(DefaultMockConfig())

	if err := repo.HealthCheck(ctx); err != nil {
		t.Errorf("HealthCheck failed: %v", err)
	}
}

func TestMockRepository_Transaction(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository(DefaultMockConfig())

	// Begin transaction
	tx, err := repo.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	// Save a snapshot within transaction
	snapshot := CostSnapshot{
		ID:                "tx-test-snapshot",
		CalculationID:     "tx-test-calculation",
		Timestamp:         time.Now(),
		TotalBillableCost: 999.0,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	txRepo := tx.Repository()
	if err := txRepo.SaveCostSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("SaveCostSnapshot in transaction failed: %v", err)
	}

	// Should be visible within transaction
	txSnapshot, err := txRepo.GetCostSnapshot(ctx, "tx-test-snapshot")
	if err != nil {
		t.Fatalf("GetCostSnapshot in transaction failed: %v", err)
	}
	if txSnapshot == nil {
		t.Fatal("Expected to find snapshot within transaction")
	}

	// Should NOT be visible outside transaction (before commit)
	_, err = repo.GetCostSnapshot(ctx, "tx-test-snapshot")
	if err == nil {
		t.Error("Snapshot should not be visible outside transaction before commit")
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Should be visible after commit
	committedSnapshot, err := repo.GetCostSnapshot(ctx, "tx-test-snapshot")
	if err != nil {
		t.Fatalf("GetCostSnapshot after commit failed: %v", err)
	}
	if committedSnapshot == nil {
		t.Fatal("Expected to find snapshot after commit")
	}
	if committedSnapshot.TotalBillableCost != 999.0 {
		t.Errorf("Expected TotalBillableCost 999.0, got %f", committedSnapshot.TotalBillableCost)
	}
}

func TestMockRepository_TransactionRollback(t *testing.T) {
	ctx := context.Background()
	repo := NewMockRepository(DefaultMockConfig())

	// Begin transaction
	tx, err := repo.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	// Save a snapshot within transaction
	snapshot := CostSnapshot{
		ID:                "rollback-test-snapshot",
		CalculationID:     "rollback-test-calculation",
		Timestamp:         time.Now(),
		TotalBillableCost: 888.0,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	txRepo := tx.Repository()
	if err := txRepo.SaveCostSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("SaveCostSnapshot in transaction failed: %v", err)
	}

	// Should be visible within transaction
	txSnapshot, err := txRepo.GetCostSnapshot(ctx, "rollback-test-snapshot")
	if err != nil {
		t.Fatalf("GetCostSnapshot in transaction failed: %v", err)
	}
	if txSnapshot == nil {
		t.Fatal("Expected to find snapshot within transaction")
	}

	// Rollback transaction
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Should NOT be visible after rollback
	_, err = repo.GetCostSnapshot(ctx, "rollback-test-snapshot")
	if err == nil {
		t.Error("Snapshot should not be visible after rollback")
	}
}

func TestMockRepository_ScenarioVariations(t *testing.T) {
	testCases := []struct {
		name     string
		scenario string
		check    func(*MockRepository)
	}{
		{
			name:     "Standard scenario",
			scenario: "standard",
			check: func(repo *MockRepository) {
				ctx := context.Background()
				snapshots, err := repo.ListCostSnapshots(ctx, CostSnapshotFilter{Limit: 5})
				if err != nil {
					t.Fatalf("ListCostSnapshots failed: %v", err)
				}
				if len(snapshots) == 0 {
					t.Error("Standard scenario should have pre-populated snapshots")
				}
			},
		},
		{
			name:     "Historical scenario",
			scenario: "historical",
			check: func(repo *MockRepository) {
				ctx := context.Background()
				dailyCosts, err := repo.ListDailyNamespaceCosts(ctx, DailyNamespaceCostFilter{Limit: 10})
				if err != nil {
					t.Fatalf("ListDailyNamespaceCosts failed: %v", err)
				}
				if len(dailyCosts) == 0 {
					t.Error("Historical scenario should have daily costs")
				}
				// Historical data should have older timestamps
				now := time.Now()
				for _, cost := range dailyCosts {
					if cost.Date.After(now) {
						t.Error("Historical data should not have future dates")
					}
				}
			},
		},
		{
			name:     "Empty scenario",
			scenario: "empty",
			check: func(repo *MockRepository) {
				ctx := context.Background()
				snapshots, err := repo.ListCostSnapshots(ctx, CostSnapshotFilter{Limit: 5})
				if err != nil {
					t.Fatalf("ListCostSnapshots failed: %v", err)
				}
				if len(snapshots) != 0 {
					t.Errorf("Empty scenario should have no snapshots, got %d", len(snapshots))
				}

				dailyCosts, err := repo.ListDailyNamespaceCosts(ctx, DailyNamespaceCostFilter{Limit: 5})
				if err != nil {
					t.Fatalf("ListDailyNamespaceCosts failed: %v", err)
				}
				if len(dailyCosts) != 0 {
					t.Errorf("Empty scenario should have no daily costs, got %d", len(dailyCosts))
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := DefaultMockConfig()
			config.Scenario = tc.scenario
			repo := NewMockRepository(config)

			tc.check(repo)
		})
	}
}

func TestMockRepository_DataSizeVariations(t *testing.T) {
	testCases := []struct {
		name         string
		dataSize     string
		minSnapshots int
		maxSnapshots int
	}{
		{"Small data size", "small", 4, 6},
		{"Medium data size", "medium", 18, 22},
		{"Large data size", "large", 48, 52},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			config := DefaultMockConfig()
			config.DataSize = tc.dataSize
			repo := NewMockRepository(config)

			snapshots, err := repo.ListCostSnapshots(ctx, CostSnapshotFilter{})
			if err != nil {
				t.Fatalf("ListCostSnapshots failed: %v", err)
			}

			if len(snapshots) < tc.minSnapshots {
				t.Errorf("Expected at least %d snapshots, got %d", tc.minSnapshots, len(snapshots))
			}
			if len(snapshots) > tc.maxSnapshots {
				t.Errorf("Expected at most %d snapshots, got %d", tc.maxSnapshots, len(snapshots))
			}
		})
	}
}

func TestMockRepository_DeterministicGeneration(t *testing.T) {
	ctx := context.Background()
	config := DefaultMockConfig()
	config.RandomSeed = 54321
	config.DataSize = "small"

	// Create two repositories with same seed
	repo1 := NewMockRepository(config)
	repo2 := NewMockRepository(config)

	snapshots1, err1 := repo1.ListCostSnapshots(ctx, CostSnapshotFilter{Limit: 5})
	if err1 != nil {
		t.Fatalf("Repo1 ListCostSnapshots failed: %v", err1)
	}

	snapshots2, err2 := repo2.ListCostSnapshots(ctx, CostSnapshotFilter{Limit: 5})
	if err2 != nil {
		t.Fatalf("Repo2 ListCostSnapshots failed: %v", err2)
	}

	// Should generate identical data with same seed
	if len(snapshots1) != len(snapshots2) {
		t.Errorf("Snapshot count mismatch: %d != %d", len(snapshots1), len(snapshots2))
	}

	for i := range snapshots1 {
		if snapshots1[i].ID != snapshots2[i].ID {
			t.Errorf("Snapshot ID mismatch at index %d: %s != %s",
				i, snapshots1[i].ID, snapshots2[i].ID)
		}
		if snapshots1[i].TotalBillableCost != snapshots2[i].TotalBillableCost {
			t.Errorf("Snapshot TotalBillableCost mismatch at index %d: %f != %f",
				i, snapshots1[i].TotalBillableCost, snapshots2[i].TotalBillableCost)
		}
	}
}
