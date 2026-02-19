package etl

import (
	"context"
	"testing"
)

func TestMockLogProcessor_Process(t *testing.T) {
	ctx := context.Background()
	mock := &MockLogProcessor{}

	batch := LogBatch{
		Entries: []LogEntry{
			{Timestamp: "2026-02-18T10:00:00Z", Level: "error", Message: "test"},
		},
		IsError: true,
	}

	err := mock.Process(ctx, batch)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if mock.ProcessedCount != 1 {
		t.Errorf("ProcessedCount = %d, want 1", mock.ProcessedCount)
	}
	if mock.LastBatch == nil || len(mock.LastBatch.Entries) != 1 {
		t.Errorf("LastBatch not set correctly")
	}
}
