// Package etl provides ETL workers for hourly and daily aggregation.
// log_processor.go: interface and Mock for evidence plane Log Processor (logs_error, logs_sampled).
package etl

import "context"

// LogProcessor defines the interface for processing logs to ClickHouse evidence plane.
// Error logs: full volume -> logs_error; Normal logs: 0.1% sample -> logs_sampled.
type LogProcessor interface {
	// Process ingests logs and writes to ClickHouse. Phase2: interface only; use Mock for tests.
	Process(ctx context.Context, batch LogBatch) error
}

// LogBatch represents a batch of logs to process.
type LogBatch struct {
	Entries []LogEntry
	IsError bool // true -> full to logs_error; false -> 0.1% sample to logs_sampled
}

// LogEntry represents a single log entry.
type LogEntry struct {
	Timestamp string
	Level     string
	Message   string
	Metadata  map[string]string
}

// MockLogProcessor is a Mock implementation of LogProcessor for tests.
type MockLogProcessor struct {
	ProcessedCount int
	LastBatch      *LogBatch
}

// Process implements LogProcessor. Phase2: Mock only; no real I/O.
func (m *MockLogProcessor) Process(ctx context.Context, batch LogBatch) error {
	m.ProcessedCount += len(batch.Entries)
	m.LastBatch = &batch
	return nil
}
