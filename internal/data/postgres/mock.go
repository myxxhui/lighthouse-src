// Package postgres provides mock implementations for testing.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/myxxhui/lighthouse-src/pkg/costmodel"
)

// MockConfig defines configuration options for the mock PostgreSQL repository.
type MockConfig struct {
	// Scenario defines the test scenario to simulate
	Scenario string `json:"scenario"` // "standard", "historical", "empty", "error"

	// DataSize defines the size of generated data sets
	DataSize string `json:"data_size"` // "small", "medium", "large"

	// InitialDataCount defines how many records to pre-populate
	InitialDataCount map[string]int `json:"initial_data_count"`

	// Namespaces to include in mock data
	Namespaces []string `json:"namespaces"`

	// Workloads per namespace
	WorkloadsPerNamespace int `json:"workloads_per_namespace"`

	// RandomSeed for deterministic generation
	RandomSeed int64 `json:"random_seed"`

	// ErrorRate controls probability of returning errors (0.0 - 1.0)
	ErrorRate float64 `json:"error_rate"`

	// LatencyMs simulates database latency in milliseconds
	LatencyMs int `json:"latency_ms"`

	// EnableTransactions simulates transaction support
	EnableTransactions bool `json:"enable_transactions"`
}

// DefaultMockConfig returns a default configuration for mock data generation.
func DefaultMockConfig() MockConfig {
	return MockConfig{
		Scenario: "standard",
		DataSize: "medium",
		InitialDataCount: map[string]int{
			"cost_snapshots":        20,
			"roi_baselines":         5,
			"daily_namespace_costs": 30,
			"hourly_workload_stats": 100,
			"metadata":              10,
		},
		Namespaces:            []string{"default", "kube-system", "monitoring", "app-prod", "app-staging"},
		WorkloadsPerNamespace: 3,
		RandomSeed:            42,
		ErrorRate:             0.0,
		LatencyMs:             5,
		EnableTransactions:    true,
	}
}

// MockRepository is a mock implementation of the PostgreSQL Repository interface.
type MockRepository struct {
	config              MockConfig
	rand                *rand.Rand
	costSnapshots       map[string]CostSnapshot
	roiBaselines        map[string]ROIBaseline
	dailyNamespaceCosts map[string]DailyNamespaceCost // key: namespace-date
	hourlyWorkloadStats map[string]HourlyWorkloadStat // key: namespace-workload-timestamp
	metadata            map[string]Metadata
}

// MockTransaction is a mock implementation of the Transaction interface.
type MockTransaction struct {
	repo       *MockRepository
	snapshots  map[string]CostSnapshot
	baselines  map[string]ROIBaseline
	dailyCosts map[string]DailyNamespaceCost
	workloads  map[string]HourlyWorkloadStat
	metadata   map[string]Metadata
	committed  bool
}

// NewMockRepository creates a new mock PostgreSQL repository with the given configuration.
func NewMockRepository(config MockConfig) *MockRepository {
	if config.RandomSeed == 0 {
		config.RandomSeed = time.Now().UnixNano()
	}

	repo := &MockRepository{
		config:              config,
		rand:                rand.New(rand.NewSource(config.RandomSeed)),
		costSnapshots:       make(map[string]CostSnapshot),
		roiBaselines:        make(map[string]ROIBaseline),
		dailyNamespaceCosts: make(map[string]DailyNamespaceCost),
		hourlyWorkloadStats: make(map[string]HourlyWorkloadStat),
		metadata:            make(map[string]Metadata),
	}

	// Pre-populate with initial data
	repo.initializeData()

	return repo
}

// SaveCostSnapshot saves a mock cost snapshot.
func (m *MockRepository) SaveCostSnapshot(ctx context.Context, snapshot CostSnapshot) error {
	if err := m.simulateLatency(); err != nil {
		return err
	}

	if m.shouldReturnError() {
		return fmt.Errorf("mock PostgreSQL error: cannot save cost snapshot")
	}

	if snapshot.ID == "" {
		snapshot.ID = fmt.Sprintf("snapshot-%d", m.rand.Int63())
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}
	snapshot.UpdatedAt = time.Now()

	m.costSnapshots[snapshot.ID] = snapshot
	return nil
}

// GetCostSnapshot retrieves a mock cost snapshot.
func (m *MockRepository) GetCostSnapshot(ctx context.Context, id string) (*CostSnapshot, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock PostgreSQL error: cannot get cost snapshot")
	}

	snapshot, exists := m.costSnapshots[id]
	if !exists {
		return nil, fmt.Errorf("cost snapshot not found: %s", id)
	}

	return &snapshot, nil
}

// ListCostSnapshots lists mock cost snapshots with filtering.
func (m *MockRepository) ListCostSnapshots(ctx context.Context, filter CostSnapshotFilter) ([]CostSnapshot, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock PostgreSQL error: cannot list cost snapshots")
	}

	var snapshots []CostSnapshot
	for _, snapshot := range m.costSnapshots {
		// Apply filters
		if filter.CalculationID != "" && snapshot.CalculationID != filter.CalculationID {
			continue
		}
		if !filter.StartTime.IsZero() && snapshot.Timestamp.Before(filter.StartTime) {
			continue
		}
		if !filter.EndTime.IsZero() && snapshot.Timestamp.After(filter.EndTime) {
			continue
		}
		if filter.MinTotalCost > 0 && snapshot.TotalBillableCost < filter.MinTotalCost {
			continue
		}
		if filter.MaxTotalCost > 0 && snapshot.TotalBillableCost > filter.MaxTotalCost {
			continue
		}

		snapshots = append(snapshots, snapshot)
	}

	// Sort by timestamp descending
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.After(snapshots[j].Timestamp)
	})

	// Apply limit and offset
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	end := len(snapshots)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	if start >= end {
		return []CostSnapshot{}, nil
	}

	return snapshots[start:end], nil
}

// DeleteCostSnapshot deletes a mock cost snapshot.
func (m *MockRepository) DeleteCostSnapshot(ctx context.Context, id string) error {
	if err := m.simulateLatency(); err != nil {
		return err
	}

	if m.shouldReturnError() {
		return fmt.Errorf("mock PostgreSQL error: cannot delete cost snapshot")
	}

	if _, exists := m.costSnapshots[id]; !exists {
		return fmt.Errorf("cost snapshot not found: %s", id)
	}

	delete(m.costSnapshots, id)
	return nil
}

// SaveROIBaseline saves a mock ROI baseline.
func (m *MockRepository) SaveROIBaseline(ctx context.Context, baseline ROIBaseline) error {
	if err := m.simulateLatency(); err != nil {
		return err
	}

	if m.shouldReturnError() {
		return fmt.Errorf("mock PostgreSQL error: cannot save ROI baseline")
	}

	if baseline.ID == "" {
		baseline.ID = fmt.Sprintf("roi-%d", m.rand.Int63())
	}
	if baseline.CreatedAt.IsZero() {
		baseline.CreatedAt = time.Now()
	}
	baseline.UpdatedAt = time.Now()

	m.roiBaselines[baseline.ID] = baseline
	return nil
}

// GetROIBaseline retrieves a mock ROI baseline.
func (m *MockRepository) GetROIBaseline(ctx context.Context, id string) (*ROIBaseline, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock PostgreSQL error: cannot get ROI baseline")
	}

	baseline, exists := m.roiBaselines[id]
	if !exists {
		return nil, fmt.Errorf("ROI baseline not found: %s", id)
	}

	return &baseline, nil
}

// ListROIBaselines lists mock ROI baselines with filtering.
func (m *MockRepository) ListROIBaselines(ctx context.Context, filter ROIBaselineFilter) ([]ROIBaseline, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock PostgreSQL error: cannot list ROI baselines")
	}

	var baselines []ROIBaseline
	for _, baseline := range m.roiBaselines {
		// Apply filters
		if filter.Name != "" && baseline.Name != filter.Name {
			continue
		}
		if filter.BaselineType != "" && baseline.BaselineType != filter.BaselineType {
			continue
		}
		if !filter.StartDate.IsZero() && baseline.TimePeriodStart.Before(filter.StartDate) {
			continue
		}
		if !filter.EndDate.IsZero() && baseline.TimePeriodEnd.After(filter.EndDate) {
			continue
		}

		baselines = append(baselines, baseline)
	}

	// Sort by creation date descending
	sort.Slice(baselines, func(i, j int) bool {
		return baselines[i].CreatedAt.After(baselines[j].CreatedAt)
	})

	// Apply limit and offset
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	end := len(baselines)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	if start >= end {
		return []ROIBaseline{}, nil
	}

	return baselines[start:end], nil
}

// DeleteROIBaseline deletes a mock ROI baseline.
func (m *MockRepository) DeleteROIBaseline(ctx context.Context, id string) error {
	if err := m.simulateLatency(); err != nil {
		return err
	}

	if m.shouldReturnError() {
		return fmt.Errorf("mock PostgreSQL error: cannot delete ROI baseline")
	}

	if _, exists := m.roiBaselines[id]; !exists {
		return fmt.Errorf("ROI baseline not found: %s", id)
	}

	delete(m.roiBaselines, id)
	return nil
}

// SaveDailyNamespaceCost saves a mock daily namespace cost.
func (m *MockRepository) SaveDailyNamespaceCost(ctx context.Context, cost DailyNamespaceCost) error {
	if err := m.simulateLatency(); err != nil {
		return err
	}

	if m.shouldReturnError() {
		return fmt.Errorf("mock PostgreSQL error: cannot save daily namespace cost")
	}

	key := fmt.Sprintf("%s-%s", cost.Namespace, cost.Date.Format("2006-01-02"))
	if cost.CreatedAt.IsZero() {
		cost.CreatedAt = time.Now()
	}

	m.dailyNamespaceCosts[key] = cost
	return nil
}

// GetDailyNamespaceCost retrieves a mock daily namespace cost.
func (m *MockRepository) GetDailyNamespaceCost(ctx context.Context, namespace string, date time.Time) (*DailyNamespaceCost, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock PostgreSQL error: cannot get daily namespace cost")
	}

	key := fmt.Sprintf("%s-%s", namespace, date.Format("2006-01-02"))
	cost, exists := m.dailyNamespaceCosts[key]
	if !exists {
		return nil, fmt.Errorf("daily namespace cost not found for %s on %s", namespace, date.Format("2006-01-02"))
	}

	return &cost, nil
}

// ListDailyNamespaceCosts lists mock daily namespace costs with filtering.
func (m *MockRepository) ListDailyNamespaceCosts(ctx context.Context, filter DailyNamespaceCostFilter) ([]DailyNamespaceCost, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock PostgreSQL error: cannot list daily namespace costs")
	}

	var costs []DailyNamespaceCost
	for _, cost := range m.dailyNamespaceCosts {
		// Apply filters
		if filter.Namespace != "" && cost.Namespace != filter.Namespace {
			continue
		}
		if !filter.StartDate.IsZero() && cost.Date.Before(filter.StartDate) {
			continue
		}
		if !filter.EndDate.IsZero() && cost.Date.After(filter.EndDate) {
			continue
		}
		if filter.MinEfficiency > 0 && cost.EfficiencyScore < filter.MinEfficiency {
			continue
		}
		if filter.MaxEfficiency > 0 && cost.EfficiencyScore > filter.MaxEfficiency {
			continue
		}

		costs = append(costs, cost)
	}

	// Sort by date descending
	sort.Slice(costs, func(i, j int) bool {
		return costs[i].Date.After(costs[j].Date)
	})

	// Apply limit and offset
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	end := len(costs)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	if start >= end {
		return []DailyNamespaceCost{}, nil
	}

	return costs[start:end], nil
}

// AggregateDailyNamespaceCosts aggregates mock daily namespace costs.
func (m *MockRepository) AggregateDailyNamespaceCosts(ctx context.Context, startDate, endDate time.Time) ([]DailyNamespaceCost, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock PostgreSQL error: cannot aggregate daily namespace costs")
	}

	// Simple aggregation by namespace
	aggregated := make(map[string]*DailyNamespaceCost)
	for _, cost := range m.dailyNamespaceCosts {
		if !startDate.IsZero() && cost.Date.Before(startDate) {
			continue
		}
		if !endDate.IsZero() && cost.Date.After(endDate) {
			continue
		}

		if agg, exists := aggregated[cost.Namespace]; exists {
			agg.BillableCost += cost.BillableCost
			agg.UsageCost += cost.UsageCost
			agg.WasteCost += cost.WasteCost
			agg.PodCount += cost.PodCount
			agg.NodeCount += cost.NodeCount
			agg.WorkloadCount += cost.WorkloadCount
			// Recalculate average efficiency
			agg.EfficiencyScore = (agg.EfficiencyScore + cost.EfficiencyScore) / 2
		} else {
			aggregated[cost.Namespace] = &DailyNamespaceCost{
				Namespace:       cost.Namespace,
				Date:            cost.Date,
				BillableCost:    cost.BillableCost,
				UsageCost:       cost.UsageCost,
				WasteCost:       cost.WasteCost,
				PodCount:        cost.PodCount,
				NodeCount:       cost.NodeCount,
				WorkloadCount:   cost.WorkloadCount,
				EfficiencyScore: cost.EfficiencyScore,
				CreatedAt:       cost.CreatedAt,
			}
		}
	}

	var result []DailyNamespaceCost
	for _, cost := range aggregated {
		result = append(result, *cost)
	}

	// Sort by namespace
	sort.Slice(result, func(i, j int) bool {
		return result[i].Namespace < result[j].Namespace
	})

	return result, nil
}

// SaveHourlyWorkloadStat saves a mock hourly workload stat.
func (m *MockRepository) SaveHourlyWorkloadStat(ctx context.Context, stat HourlyWorkloadStat) error {
	if err := m.simulateLatency(); err != nil {
		return err
	}

	if m.shouldReturnError() {
		return fmt.Errorf("mock PostgreSQL error: cannot save hourly workload stat")
	}

	key := fmt.Sprintf("%s-%s-%s", stat.Namespace, stat.WorkloadName, stat.Timestamp.Format("2006-01-02-15"))
	m.hourlyWorkloadStats[key] = stat
	return nil
}

// GetHourlyWorkloadStat retrieves a mock hourly workload stat.
func (m *MockRepository) GetHourlyWorkloadStat(ctx context.Context, namespace, workloadName string, timestamp time.Time) (*HourlyWorkloadStat, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock PostgreSQL error: cannot get hourly workload stat")
	}

	key := fmt.Sprintf("%s-%s-%s", namespace, workloadName, timestamp.Format("2006-01-02-15"))
	stat, exists := m.hourlyWorkloadStats[key]
	if !exists {
		return nil, fmt.Errorf("hourly workload stat not found for %s/%s at %s", namespace, workloadName, timestamp.Format("2006-01-02 15:04"))
	}

	return &stat, nil
}

// ListHourlyWorkloadStats lists mock hourly workload stats with filtering.
func (m *MockRepository) ListHourlyWorkloadStats(ctx context.Context, filter HourlyWorkloadStatFilter) ([]HourlyWorkloadStat, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock PostgreSQL error: cannot list hourly workload stats")
	}

	var stats []HourlyWorkloadStat
	for _, stat := range m.hourlyWorkloadStats {
		// Apply filters
		if filter.Namespace != "" && stat.Namespace != filter.Namespace {
			continue
		}
		if filter.WorkloadName != "" && stat.WorkloadName != filter.WorkloadName {
			continue
		}
		if filter.NodeName != "" && stat.NodeName != filter.NodeName {
			continue
		}
		if !filter.StartTime.IsZero() && stat.Timestamp.Before(filter.StartTime) {
			continue
		}
		if !filter.EndTime.IsZero() && stat.Timestamp.After(filter.EndTime) {
			continue
		}

		stats = append(stats, stat)
	}

	// Sort by timestamp descending
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Timestamp.After(stats[j].Timestamp)
	})

	// Apply limit and offset
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	end := len(stats)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	if start >= end {
		return []HourlyWorkloadStat{}, nil
	}

	return stats[start:end], nil
}

// AggregateHourlyWorkloadStats aggregates mock hourly workload stats.
func (m *MockRepository) AggregateHourlyWorkloadStats(ctx context.Context, startTime, endTime time.Time) ([]HourlyWorkloadStat, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock PostgreSQL error: cannot aggregate hourly workload stats")
	}

	// Simple aggregation by workload
	aggregated := make(map[string]*HourlyWorkloadStat)
	for _, stat := range m.hourlyWorkloadStats {
		if !startTime.IsZero() && stat.Timestamp.Before(startTime) {
			continue
		}
		if !endTime.IsZero() && stat.Timestamp.After(endTime) {
			continue
		}

		key := fmt.Sprintf("%s-%s", stat.Namespace, stat.WorkloadName)
		if agg, exists := aggregated[key]; exists {
			agg.CPURequest += stat.CPURequest
			agg.CPUUsageP95 += stat.CPUUsageP95
			agg.MemRequest += stat.MemRequest
			agg.MemUsageP95 += stat.MemUsageP95
			agg.CPUBillableCost += stat.CPUBillableCost
			agg.CPUUsageCost += stat.CPUUsageCost
			agg.CPUWasteCost += stat.CPUWasteCost
			agg.MemBillableCost += stat.MemBillableCost
			agg.MemUsageCost += stat.MemUsageCost
			agg.MemWasteCost += stat.MemWasteCost
			agg.TotalBillableCost += stat.TotalBillableCost
			agg.TotalUsageCost += stat.TotalUsageCost
			agg.TotalWasteCost += stat.TotalWasteCost
		} else {
			aggregated[key] = &HourlyWorkloadStat{
				Namespace:         stat.Namespace,
				WorkloadName:      stat.WorkloadName,
				WorkloadType:      stat.WorkloadType,
				NodeName:          stat.NodeName,
				PodName:           stat.PodName,
				Timestamp:         stat.Timestamp,
				CPURequest:        stat.CPURequest,
				CPUUsageP95:       stat.CPUUsageP95,
				MemRequest:        stat.MemRequest,
				MemUsageP95:       stat.MemUsageP95,
				CPUBillableCost:   stat.CPUBillableCost,
				CPUUsageCost:      stat.CPUUsageCost,
				CPUWasteCost:      stat.CPUWasteCost,
				MemBillableCost:   stat.MemBillableCost,
				MemUsageCost:      stat.MemUsageCost,
				MemWasteCost:      stat.MemWasteCost,
				TotalBillableCost: stat.TotalBillableCost,
				TotalUsageCost:    stat.TotalUsageCost,
				TotalWasteCost:    stat.TotalWasteCost,
			}
		}
	}

	var result []HourlyWorkloadStat
	for _, stat := range aggregated {
		result = append(result, *stat)
	}

	// Sort by namespace and workload name
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		return result[i].WorkloadName < result[j].WorkloadName
	})

	return result, nil
}

// SaveMetadata saves mock metadata.
func (m *MockRepository) SaveMetadata(ctx context.Context, metadata Metadata) error {
	if err := m.simulateLatency(); err != nil {
		return err
	}

	if m.shouldReturnError() {
		return fmt.Errorf("mock PostgreSQL error: cannot save metadata")
	}

	if metadata.CreatedAt.IsZero() {
		metadata.CreatedAt = time.Now()
	}
	metadata.UpdatedAt = time.Now()

	m.metadata[metadata.Key] = metadata
	return nil
}

// GetMetadata retrieves mock metadata.
func (m *MockRepository) GetMetadata(ctx context.Context, key string) (*Metadata, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock PostgreSQL error: cannot get metadata")
	}

	metadata, exists := m.metadata[key]
	if !exists {
		return nil, fmt.Errorf("metadata not found: %s", key)
	}

	return &metadata, nil
}

// ListMetadata lists mock metadata with filtering.
func (m *MockRepository) ListMetadata(ctx context.Context, filter MetadataFilter) ([]Metadata, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock PostgreSQL error: cannot list metadata")
	}

	var result []Metadata
	for key, metadata := range m.metadata {
		// Apply filters
		if filter.KeyPrefix != "" && len(key) >= len(filter.KeyPrefix) && key[:len(filter.KeyPrefix)] != filter.KeyPrefix {
			continue
		}
		if filter.CreatedBy != "" && metadata.CreatedBy != filter.CreatedBy {
			continue
		}

		result = append(result, metadata)
	}

	// Sort by key
	sort.Slice(result, func(i, j int) bool {
		return result[i].Key < result[j].Key
	})

	// Apply limit and offset
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	end := len(result)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	if start >= end {
		return []Metadata{}, nil
	}

	return result[start:end], nil
}

// DeleteMetadata deletes mock metadata.
func (m *MockRepository) DeleteMetadata(ctx context.Context, key string) error {
	if err := m.simulateLatency(); err != nil {
		return err
	}

	if m.shouldReturnError() {
		return fmt.Errorf("mock PostgreSQL error: cannot delete metadata")
	}

	if _, exists := m.metadata[key]; !exists {
		return fmt.Errorf("metadata not found: %s", key)
	}

	delete(m.metadata, key)
	return nil
}

// HealthCheck always returns nil (healthy) for mock repository.
func (m *MockRepository) HealthCheck(ctx context.Context) error {
	if m.shouldReturnError() {
		return fmt.Errorf("mock PostgreSQL health check failed")
	}
	return nil
}

// BeginTx starts a mock transaction.
func (m *MockRepository) BeginTx(ctx context.Context) (Transaction, error) {
	if err := m.simulateLatency(); err != nil {
		return nil, err
	}

	if m.shouldReturnError() {
		return nil, fmt.Errorf("mock PostgreSQL error: cannot begin transaction")
	}

	if !m.config.EnableTransactions {
		return nil, errors.New("transactions not enabled in mock configuration")
	}

	// Create copies of current data for transaction isolation
	txSnapshots := make(map[string]CostSnapshot)
	for k, v := range m.costSnapshots {
		txSnapshots[k] = v
	}

	txBaselines := make(map[string]ROIBaseline)
	for k, v := range m.roiBaselines {
		txBaselines[k] = v
	}

	txDailyCosts := make(map[string]DailyNamespaceCost)
	for k, v := range m.dailyNamespaceCosts {
		txDailyCosts[k] = v
	}

	txWorkloads := make(map[string]HourlyWorkloadStat)
	for k, v := range m.hourlyWorkloadStats {
		txWorkloads[k] = v
	}

	txMetadata := make(map[string]Metadata)
	for k, v := range m.metadata {
		txMetadata[k] = v
	}

	tx := &MockTransaction{
		repo:       m,
		snapshots:  txSnapshots,
		baselines:  txBaselines,
		dailyCosts: txDailyCosts,
		workloads:  txWorkloads,
		metadata:   txMetadata,
		committed:  false,
	}

	return tx, nil
}

// Commit commits the mock transaction.
func (tx *MockTransaction) Commit() error {
	if tx.committed {
		return errors.New("transaction already committed")
	}

	// Apply transaction changes to repository
	tx.repo.costSnapshots = tx.snapshots
	tx.repo.roiBaselines = tx.baselines
	tx.repo.dailyNamespaceCosts = tx.dailyCosts
	tx.repo.hourlyWorkloadStats = tx.workloads
	tx.repo.metadata = tx.metadata

	tx.committed = true
	return nil
}

// Rollback rolls back the mock transaction.
func (tx *MockTransaction) Rollback() error {
	if tx.committed {
		return errors.New("transaction already committed")
	}
	// Nothing to do, transaction changes are discarded
	return nil
}

// Repository returns the transaction's repository interface.
func (tx *MockTransaction) Repository() Repository {
	// Return a wrapper that uses transaction data
	return &transactionRepository{tx: tx}
}

// transactionRepository is a wrapper that uses transaction data.
type transactionRepository struct {
	tx *MockTransaction
}

func (tr *transactionRepository) SaveCostSnapshot(ctx context.Context, snapshot CostSnapshot) error {
	if snapshot.ID == "" {
		snapshot.ID = fmt.Sprintf("tx-snapshot-%d", tr.tx.repo.rand.Int63())
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}
	snapshot.UpdatedAt = time.Now()

	tr.tx.snapshots[snapshot.ID] = snapshot
	return nil
}

func (tr *transactionRepository) GetCostSnapshot(ctx context.Context, id string) (*CostSnapshot, error) {
	snapshot, exists := tr.tx.snapshots[id]
	if !exists {
		return nil, fmt.Errorf("cost snapshot not found: %s", id)
	}
	return &snapshot, nil
}

func (tr *transactionRepository) ListCostSnapshots(ctx context.Context, filter CostSnapshotFilter) ([]CostSnapshot, error) {
	var snapshots []CostSnapshot
	for _, snapshot := range tr.tx.snapshots {
		if filter.CalculationID != "" && snapshot.CalculationID != filter.CalculationID {
			continue
		}
		if !filter.StartTime.IsZero() && snapshot.Timestamp.Before(filter.StartTime) {
			continue
		}
		if !filter.EndTime.IsZero() && snapshot.Timestamp.After(filter.EndTime) {
			continue
		}
		if filter.MinTotalCost > 0 && snapshot.TotalBillableCost < filter.MinTotalCost {
			continue
		}
		if filter.MaxTotalCost > 0 && snapshot.TotalBillableCost > filter.MaxTotalCost {
			continue
		}
		snapshots = append(snapshots, snapshot)
	}
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.After(snapshots[j].Timestamp)
	})
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	end := len(snapshots)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	if start >= end {
		return []CostSnapshot{}, nil
	}
	return snapshots[start:end], nil
}

func (tr *transactionRepository) DeleteCostSnapshot(ctx context.Context, id string) error {
	if _, exists := tr.tx.snapshots[id]; !exists {
		return fmt.Errorf("cost snapshot not found: %s", id)
	}
	delete(tr.tx.snapshots, id)
	return nil
}

func (tr *transactionRepository) SaveROIBaseline(ctx context.Context, baseline ROIBaseline) error {
	if baseline.ID == "" {
		baseline.ID = fmt.Sprintf("tx-roi-%d", tr.tx.repo.rand.Int63())
	}
	if baseline.CreatedAt.IsZero() {
		baseline.CreatedAt = time.Now()
	}
	baseline.UpdatedAt = time.Now()
	tr.tx.baselines[baseline.ID] = baseline
	return nil
}

func (tr *transactionRepository) GetROIBaseline(ctx context.Context, id string) (*ROIBaseline, error) {
	baseline, exists := tr.tx.baselines[id]
	if !exists {
		return nil, fmt.Errorf("ROI baseline not found: %s", id)
	}
	return &baseline, nil
}

func (tr *transactionRepository) ListROIBaselines(ctx context.Context, filter ROIBaselineFilter) ([]ROIBaseline, error) {
	var baselines []ROIBaseline
	for _, baseline := range tr.tx.baselines {
		if filter.Name != "" && baseline.Name != filter.Name {
			continue
		}
		if filter.BaselineType != "" && baseline.BaselineType != filter.BaselineType {
			continue
		}
		if !filter.StartDate.IsZero() && baseline.TimePeriodStart.Before(filter.StartDate) {
			continue
		}
		if !filter.EndDate.IsZero() && baseline.TimePeriodEnd.After(filter.EndDate) {
			continue
		}
		baselines = append(baselines, baseline)
	}
	sort.Slice(baselines, func(i, j int) bool {
		return baselines[i].CreatedAt.After(baselines[j].CreatedAt)
	})
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	end := len(baselines)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	if start >= end {
		return []ROIBaseline{}, nil
	}
	return baselines[start:end], nil
}

func (tr *transactionRepository) DeleteROIBaseline(ctx context.Context, id string) error {
	if _, exists := tr.tx.baselines[id]; !exists {
		return fmt.Errorf("ROI baseline not found: %s", id)
	}
	delete(tr.tx.baselines, id)
	return nil
}

func (tr *transactionRepository) SaveDailyNamespaceCost(ctx context.Context, cost DailyNamespaceCost) error {
	key := fmt.Sprintf("%s-%s", cost.Namespace, cost.Date.Format("2006-01-02"))
	if cost.CreatedAt.IsZero() {
		cost.CreatedAt = time.Now()
	}
	tr.tx.dailyCosts[key] = cost
	return nil
}

func (tr *transactionRepository) GetDailyNamespaceCost(ctx context.Context, namespace string, date time.Time) (*DailyNamespaceCost, error) {
	key := fmt.Sprintf("%s-%s", namespace, date.Format("2006-01-02"))
	cost, exists := tr.tx.dailyCosts[key]
	if !exists {
		return nil, fmt.Errorf("daily namespace cost not found for %s on %s", namespace, date.Format("2006-01-02"))
	}
	return &cost, nil
}

func (tr *transactionRepository) ListDailyNamespaceCosts(ctx context.Context, filter DailyNamespaceCostFilter) ([]DailyNamespaceCost, error) {
	var costs []DailyNamespaceCost
	for _, cost := range tr.tx.dailyCosts {
		if filter.Namespace != "" && cost.Namespace != filter.Namespace {
			continue
		}
		if !filter.StartDate.IsZero() && cost.Date.Before(filter.StartDate) {
			continue
		}
		if !filter.EndDate.IsZero() && cost.Date.After(filter.EndDate) {
			continue
		}
		if filter.MinEfficiency > 0 && cost.EfficiencyScore < filter.MinEfficiency {
			continue
		}
		if filter.MaxEfficiency > 0 && cost.EfficiencyScore > filter.MaxEfficiency {
			continue
		}
		costs = append(costs, cost)
	}
	sort.Slice(costs, func(i, j int) bool {
		return costs[i].Date.After(costs[j].Date)
	})
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	end := len(costs)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	if start >= end {
		return []DailyNamespaceCost{}, nil
	}
	return costs[start:end], nil
}

func (tr *transactionRepository) AggregateDailyNamespaceCosts(ctx context.Context, startDate, endDate time.Time) ([]DailyNamespaceCost, error) {
	aggregated := make(map[string]*DailyNamespaceCost)
	for _, cost := range tr.tx.dailyCosts {
		if !startDate.IsZero() && cost.Date.Before(startDate) {
			continue
		}
		if !endDate.IsZero() && cost.Date.After(endDate) {
			continue
		}
		if agg, exists := aggregated[cost.Namespace]; exists {
			agg.BillableCost += cost.BillableCost
			agg.UsageCost += cost.UsageCost
			agg.WasteCost += cost.WasteCost
			agg.PodCount += cost.PodCount
			agg.NodeCount += cost.NodeCount
			agg.WorkloadCount += cost.WorkloadCount
			agg.EfficiencyScore = (agg.EfficiencyScore + cost.EfficiencyScore) / 2
		} else {
			aggregated[cost.Namespace] = &DailyNamespaceCost{
				Namespace:       cost.Namespace,
				Date:            cost.Date,
				BillableCost:    cost.BillableCost,
				UsageCost:       cost.UsageCost,
				WasteCost:       cost.WasteCost,
				PodCount:        cost.PodCount,
				NodeCount:       cost.NodeCount,
				WorkloadCount:   cost.WorkloadCount,
				EfficiencyScore: cost.EfficiencyScore,
				CreatedAt:       cost.CreatedAt,
			}
		}
	}
	var result []DailyNamespaceCost
	for _, cost := range aggregated {
		result = append(result, *cost)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Namespace < result[j].Namespace
	})
	return result, nil
}

func (tr *transactionRepository) SaveHourlyWorkloadStat(ctx context.Context, stat HourlyWorkloadStat) error {
	key := fmt.Sprintf("%s-%s-%s", stat.Namespace, stat.WorkloadName, stat.Timestamp.Format("2006-01-02-15"))
	tr.tx.workloads[key] = stat
	return nil
}

func (tr *transactionRepository) GetHourlyWorkloadStat(ctx context.Context, namespace, workloadName string, timestamp time.Time) (*HourlyWorkloadStat, error) {
	key := fmt.Sprintf("%s-%s-%s", namespace, workloadName, timestamp.Format("2006-01-02-15"))
	stat, exists := tr.tx.workloads[key]
	if !exists {
		return nil, fmt.Errorf("hourly workload stat not found for %s/%s at %s", namespace, workloadName, timestamp.Format("2006-01-02 15:04"))
	}
	return &stat, nil
}

func (tr *transactionRepository) ListHourlyWorkloadStats(ctx context.Context, filter HourlyWorkloadStatFilter) ([]HourlyWorkloadStat, error) {
	var stats []HourlyWorkloadStat
	for _, stat := range tr.tx.workloads {
		if filter.Namespace != "" && stat.Namespace != filter.Namespace {
			continue
		}
		if filter.WorkloadName != "" && stat.WorkloadName != filter.WorkloadName {
			continue
		}
		if filter.NodeName != "" && stat.NodeName != filter.NodeName {
			continue
		}
		if !filter.StartTime.IsZero() && stat.Timestamp.Before(filter.StartTime) {
			continue
		}
		if !filter.EndTime.IsZero() && stat.Timestamp.After(filter.EndTime) {
			continue
		}
		stats = append(stats, stat)
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Timestamp.After(stats[j].Timestamp)
	})
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	end := len(stats)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	if start >= end {
		return []HourlyWorkloadStat{}, nil
	}
	return stats[start:end], nil
}

func (tr *transactionRepository) AggregateHourlyWorkloadStats(ctx context.Context, startTime, endTime time.Time) ([]HourlyWorkloadStat, error) {
	aggregated := make(map[string]*HourlyWorkloadStat)
	for _, stat := range tr.tx.workloads {
		if !startTime.IsZero() && stat.Timestamp.Before(startTime) {
			continue
		}
		if !endTime.IsZero() && stat.Timestamp.After(endTime) {
			continue
		}
		key := fmt.Sprintf("%s-%s", stat.Namespace, stat.WorkloadName)
		if agg, exists := aggregated[key]; exists {
			agg.CPURequest += stat.CPURequest
			agg.CPUUsageP95 += stat.CPUUsageP95
			agg.MemRequest += stat.MemRequest
			agg.MemUsageP95 += stat.MemUsageP95
			agg.CPUBillableCost += stat.CPUBillableCost
			agg.CPUUsageCost += stat.CPUUsageCost
			agg.CPUWasteCost += stat.CPUWasteCost
			agg.MemBillableCost += stat.MemBillableCost
			agg.MemUsageCost += stat.MemUsageCost
			agg.MemWasteCost += stat.MemWasteCost
			agg.TotalBillableCost += stat.TotalBillableCost
			agg.TotalUsageCost += stat.TotalUsageCost
			agg.TotalWasteCost += stat.TotalWasteCost
		} else {
			aggregated[key] = &HourlyWorkloadStat{
				Namespace:         stat.Namespace,
				WorkloadName:      stat.WorkloadName,
				WorkloadType:      stat.WorkloadType,
				NodeName:          stat.NodeName,
				PodName:           stat.PodName,
				Timestamp:         stat.Timestamp,
				CPURequest:        stat.CPURequest,
				CPUUsageP95:       stat.CPUUsageP95,
				MemRequest:        stat.MemRequest,
				MemUsageP95:       stat.MemUsageP95,
				CPUBillableCost:   stat.CPUBillableCost,
				CPUUsageCost:      stat.CPUUsageCost,
				CPUWasteCost:      stat.CPUWasteCost,
				MemBillableCost:   stat.MemBillableCost,
				MemUsageCost:      stat.MemUsageCost,
				MemWasteCost:      stat.MemWasteCost,
				TotalBillableCost: stat.TotalBillableCost,
				TotalUsageCost:    stat.TotalUsageCost,
				TotalWasteCost:    stat.TotalWasteCost,
			}
		}
	}
	var result []HourlyWorkloadStat
	for _, stat := range aggregated {
		result = append(result, *stat)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		return result[i].WorkloadName < result[j].WorkloadName
	})
	return result, nil
}

func (tr *transactionRepository) SaveMetadata(ctx context.Context, metadata Metadata) error {
	if metadata.CreatedAt.IsZero() {
		metadata.CreatedAt = time.Now()
	}
	metadata.UpdatedAt = time.Now()
	tr.tx.metadata[metadata.Key] = metadata
	return nil
}

func (tr *transactionRepository) GetMetadata(ctx context.Context, key string) (*Metadata, error) {
	metadata, exists := tr.tx.metadata[key]
	if !exists {
		return nil, fmt.Errorf("metadata not found: %s", key)
	}
	return &metadata, nil
}

func (tr *transactionRepository) ListMetadata(ctx context.Context, filter MetadataFilter) ([]Metadata, error) {
	var result []Metadata
	for key, metadata := range tr.tx.metadata {
		if filter.KeyPrefix != "" && len(key) >= len(filter.KeyPrefix) && key[:len(filter.KeyPrefix)] != filter.KeyPrefix {
			continue
		}
		if filter.CreatedBy != "" && metadata.CreatedBy != filter.CreatedBy {
			continue
		}
		result = append(result, metadata)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Key < result[j].Key
	})
	start := filter.Offset
	if start < 0 {
		start = 0
	}
	end := len(result)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	if start >= end {
		return []Metadata{}, nil
	}
	return result[start:end], nil
}

func (tr *transactionRepository) DeleteMetadata(ctx context.Context, key string) error {
	if _, exists := tr.tx.metadata[key]; !exists {
		return fmt.Errorf("metadata not found: %s", key)
	}
	delete(tr.tx.metadata, key)
	return nil
}

func (tr *transactionRepository) HealthCheck(ctx context.Context) error {
	return nil
}

func (tr *transactionRepository) BeginTx(ctx context.Context) (Transaction, error) {
	return nil, errors.New("nested transactions not supported in mock")
}

// Helper methods for MockRepository

func (m *MockRepository) simulateLatency() error {
	if m.config.LatencyMs > 0 {
		time.Sleep(time.Duration(m.config.LatencyMs) * time.Millisecond)
	}
	return nil
}

func (m *MockRepository) shouldReturnError() bool {
	if m.config.ErrorRate <= 0.0 {
		return false
	}
	return m.rand.Float64() < m.config.ErrorRate
}

func (m *MockRepository) initializeData() {
	if m.config.Scenario == "empty" {
		return
	}

	// Initialize cost snapshots
	for i := 0; i < m.config.InitialDataCount["cost_snapshots"]; i++ {
		snapshot := m.generateCostSnapshot(i)
		m.costSnapshots[snapshot.ID] = snapshot
	}

	// Initialize ROI baselines
	for i := 0; i < m.config.InitialDataCount["roi_baselines"]; i++ {
		baseline := m.generateROIBaseline(i)
		m.roiBaselines[baseline.ID] = baseline
	}

	// Initialize daily namespace costs
	for i := 0; i < m.config.InitialDataCount["daily_namespace_costs"]; i++ {
		cost := m.generateDailyNamespaceCost(i)
		key := fmt.Sprintf("%s-%s", cost.Namespace, cost.Date.Format("2006-01-02"))
		m.dailyNamespaceCosts[key] = cost
	}

	// Initialize hourly workload stats
	for i := 0; i < m.config.InitialDataCount["hourly_workload_stats"]; i++ {
		stat := m.generateHourlyWorkloadStat(i)
		key := fmt.Sprintf("%s-%s-%s", stat.Namespace, stat.WorkloadName, stat.Timestamp.Format("2006-01-02-15"))
		m.hourlyWorkloadStats[key] = stat
	}

	// Initialize metadata
	for i := 0; i < m.config.InitialDataCount["metadata"]; i++ {
		metadata := m.generateMetadata(i)
		m.metadata[metadata.Key] = metadata
	}
}

func (m *MockRepository) generateCostSnapshot(index int) CostSnapshot {
	now := time.Now()
	daysAgo := m.rand.Intn(30)
	timestamp := now.Add(-time.Duration(daysAgo) * 24 * time.Hour)

	// Generate some resource results
	var resourceResults []costmodel.CostResult
	for i := 0; i < 5+m.rand.Intn(10); i++ {
		result := costmodel.CostResult{
			CPUBillableCost:        100 + m.rand.Float64()*500,
			CPUUsageCost:           50 + m.rand.Float64()*300,
			CPUWasteCost:           20 + m.rand.Float64()*100,
			CPUEfficiencyScore:     0.5 + m.rand.Float64()*0.5,
			MemBillableCost:        200 + m.rand.Float64()*800,
			MemUsageCost:           100 + m.rand.Float64()*400,
			MemWasteCost:           50 + m.rand.Float64()*200,
			MemEfficiencyScore:     0.4 + m.rand.Float64()*0.6,
			TotalBillableCost:      300 + m.rand.Float64()*1300,
			TotalUsageCost:         150 + m.rand.Float64()*700,
			TotalWasteCost:         70 + m.rand.Float64()*300,
			OverallEfficiencyScore: 0.45 + m.rand.Float64()*0.55,
			OverallGrade:           costmodel.EfficiencyGrade("Healthy"),
		}
		resourceResults = append(resourceResults, result)
	}

	return CostSnapshot{
		ID:                     fmt.Sprintf("snapshot-%d", index),
		CalculationID:          fmt.Sprintf("calc-%d", index),
		Timestamp:              timestamp,
		TimeRangeStart:         timestamp.Add(-24 * time.Hour),
		TimeRangeEnd:           timestamp,
		ResourceResults:        resourceResults,
		AggregatedResults:      make(map[costmodel.AggregationLevel][]costmodel.AggregationResult),
		TotalBillableCost:      1000 + m.rand.Float64()*5000,
		TotalUsageCost:         500 + m.rand.Float64()*2500,
		TotalWasteCost:         200 + m.rand.Float64()*1000,
		OverallEfficiencyScore: 0.6 + m.rand.Float64()*0.4,
		ZombieCount:            m.rand.Intn(5),
		OverProvisionedCount:   m.rand.Intn(10),
		HealthyCount:           15 + m.rand.Intn(20),
		RiskCount:              m.rand.Intn(3),
		Metadata:               map[string]interface{}{"generated_by": "mock", "index": index},
		CreatedAt:              timestamp,
		UpdatedAt:              timestamp,
	}
}

func (m *MockRepository) generateROIBaseline(index int) ROIBaseline {
	baselineTypes := []string{"historical", "target", "industry"}
	baselineType := baselineTypes[m.rand.Intn(len(baselineTypes))]

	now := time.Now()
	startDate := now.Add(-time.Duration(30+m.rand.Intn(60)) * 24 * time.Hour)
	endDate := startDate.Add(time.Duration(30) * 24 * time.Hour)

	metrics := map[string]float64{
		"efficiency_score": 0.7 + m.rand.Float64()*0.3,
		"waste_percentage": 0.1 + m.rand.Float64()*0.2,
		"cost_per_pod":     50 + m.rand.Float64()*150,
		"utilization_rate": 0.6 + m.rand.Float64()*0.3,
	}

	return ROIBaseline{
		ID:              fmt.Sprintf("roi-%d", index),
		Name:            fmt.Sprintf("%s-baseline-%d", baselineType, index),
		Description:     fmt.Sprintf("Mock %s baseline for testing", baselineType),
		BaselineType:    baselineType,
		TimePeriodStart: startDate,
		TimePeriodEnd:   endDate,
		Metrics:         metrics,
		ReferenceData:   map[string]interface{}{"source": "mock", "confidence": 0.9},
		CreatedBy:       "mock-user",
		CreatedAt:       now.Add(-time.Duration(index) * 24 * time.Hour),
		UpdatedAt:       now.Add(-time.Duration(index) * 24 * time.Hour),
	}
}

func (m *MockRepository) generateDailyNamespaceCost(index int) DailyNamespaceCost {
	namespaceIdx := index % len(m.config.Namespaces)
	namespace := m.config.Namespaces[namespaceIdx]

	daysAgo := m.rand.Intn(60)
	date := time.Now().Add(-time.Duration(daysAgo) * 24 * time.Hour).Truncate(24 * time.Hour)

	return DailyNamespaceCost{
		Namespace:       namespace,
		Date:            date,
		BillableCost:    1000 + m.rand.Float64()*5000,
		UsageCost:       400 + m.rand.Float64()*2000,
		WasteCost:       100 + m.rand.Float64()*500,
		PodCount:        5 + m.rand.Intn(20),
		NodeCount:       1 + m.rand.Intn(5),
		WorkloadCount:   3 + m.rand.Intn(10),
		EfficiencyScore: 0.5 + m.rand.Float64()*0.5,
		CreatedAt:       date,
	}
}

func (m *MockRepository) generateHourlyWorkloadStat(index int) HourlyWorkloadStat {
	namespaceIdx := index % len(m.config.Namespaces)
	namespace := m.config.Namespaces[namespaceIdx]
	workloadNum := (index / len(m.config.Namespaces)) % m.config.WorkloadsPerNamespace

	hoursAgo := m.rand.Intn(168) // Up to 1 week
	timestamp := time.Now().Add(-time.Duration(hoursAgo) * time.Hour).Truncate(time.Hour)

	return HourlyWorkloadStat{
		Namespace:         namespace,
		WorkloadName:      fmt.Sprintf("workload-%d", workloadNum),
		WorkloadType:      "Deployment",
		NodeName:          fmt.Sprintf("node-%d", 1+m.rand.Intn(4)),
		PodName:           fmt.Sprintf("pod-%d", index%10),
		Timestamp:         timestamp,
		CPURequest:        0.5 + m.rand.Float64()*3.0,
		CPUUsageP95:       0.2 + m.rand.Float64()*1.5,
		MemRequest:        int64(512*1024*1024 + m.rand.Intn(2*1024*1024*1024)), // 512MB - 2.5GB
		MemUsageP95:       int64(256*1024*1024 + m.rand.Intn(1*1024*1024*1024)), // 256MB - 1.25GB
		CPUBillableCost:   10 + m.rand.Float64()*50,
		CPUUsageCost:      4 + m.rand.Float64()*25,
		CPUWasteCost:      1 + m.rand.Float64()*10,
		MemBillableCost:   20 + m.rand.Float64()*100,
		MemUsageCost:      8 + m.rand.Float64()*50,
		MemWasteCost:      int64(2 + m.rand.Intn(10)),
		TotalBillableCost: 30 + m.rand.Float64()*150,
		TotalUsageCost:    12 + m.rand.Float64()*75,
		TotalWasteCost:    3 + m.rand.Float64()*20,
	}
}

func (m *MockRepository) generateMetadata(index int) Metadata {
	keys := []string{
		"system.version",
		"last_calculation_time",
		"notification_settings",
		"user_preferences",
		"export_config",
	}

	key := keys[index%len(keys)]
	if index >= len(keys) {
		key = fmt.Sprintf("custom.key.%d", index)
	}

	return Metadata{
		Key: key,
		Value: map[string]interface{}{
			"value":     fmt.Sprintf("mock-value-%d", index),
			"timestamp": time.Now(),
			"index":     index,
		},
		Description: fmt.Sprintf("Mock metadata for %s", key),
		CreatedBy:   "mock-system",
		CreatedAt:   time.Now().Add(-time.Duration(index) * time.Hour),
		UpdatedAt:   time.Now().Add(-time.Duration(index/2) * time.Hour),
	}
}
